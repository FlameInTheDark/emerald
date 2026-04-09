package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/automator/internal/assistants"
	"github.com/FlameInTheDark/automator/internal/db/models"
	"github.com/FlameInTheDark/automator/internal/db/query"
	"github.com/FlameInTheDark/automator/internal/llm"
)

type EditorAssistantHandler struct {
	providerStore     *query.LLMProviderStore
	assistantProfiles *assistants.Store
}

type editorAssistantRequest struct {
	ProviderID string                          `json:"provider_id,omitempty"`
	Mode       string                          `json:"mode"`
	Message    string                          `json:"message"`
	Messages   []editorAssistantHistoryMessage `json:"messages"`
	Pipeline   assistants.PipelineSnapshot     `json:"pipeline"`
	Selection  assistants.SelectionSnapshot    `json:"selection"`
}

type editorAssistantHistoryMessage struct {
	Role        string           `json:"role"`
	Content     string           `json:"content"`
	ToolCalls   []llm.ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []llm.ToolResult `json:"tool_results,omitempty"`
}

type editorAssistantResponsePayload struct {
	Content     string                             `json:"content"`
	ToolCalls   []llm.ToolCall                     `json:"tool_calls,omitempty"`
	ToolResults []llm.ToolResult                   `json:"tool_results,omitempty"`
	Usage       llm.Usage                          `json:"usage"`
	Operations  []assistants.LivePipelineOperation `json:"operations,omitempty"`
}

type editorAssistantToolExecutor struct {
	snapshot assistants.PipelineSnapshot
	enabled  bool
}

func NewEditorAssistantHandler(
	providerStore *query.LLMProviderStore,
	assistantProfiles *assistants.Store,
) *EditorAssistantHandler {
	return &EditorAssistantHandler{
		providerStore:     providerStore,
		assistantProfiles: assistantProfiles,
	}
}

func (h *EditorAssistantHandler) ChatStream(c *fiber.Ctx) error {
	if _, ok := currentAuthSession(c); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	var req editorAssistantRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "message is required"})
	}

	mode := strings.TrimSpace(strings.ToLower(req.Mode))
	if mode != "ask" && mode != "edit" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "mode must be ask or edit"})
	}

	snapshot, err := assistants.NormalizePipelineSnapshot(req.Pipeline)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	profile, err := h.assistantProfiles.Get(c.Context(), assistants.ScopePipelineEditor)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	_, providerConfig, err := h.resolveProvider(c.Context(), req.ProviderID)
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(err.Error(), "provider") {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	provider, err := llm.NewProvider(providerConfig)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("failed to initialize provider: %v", err)})
	}

	modelMessages, err := h.buildModelMessages(profile, mode, req.Messages, req.Message, snapshot, req.Selection)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	streamCtx := context.Background()
	if userCtx := c.UserContext(); userCtx != nil {
		streamCtx = userCtx
	}

	toolExecutor := newEditorAssistantToolExecutor(snapshot, mode == "edit")

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(writer *bufio.Writer) {
		resp, toolCalls, toolResults, _, err := runToolChatStream(
			streamCtx,
			provider,
			providerConfig.Model,
			modelMessages,
			toolExecutor,
			func(event llm.ToolChatEvent) error {
				switch event.Type {
				case llm.ToolChatEventContentDelta:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{Type: "assistant_delta", Delta: event.Delta})
				case llm.ToolChatEventToolStarted:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{Type: "tool_started", ToolCall: event.ToolCall})
				case llm.ToolChatEventToolFinished:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{Type: "tool_finished", ToolCall: event.ToolCall, ToolResult: event.ToolResult})
				case llm.ToolChatEventUsage:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{Type: "usage", Usage: event.Usage})
				default:
					return nil
				}
			},
		)
		if err != nil {
			_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
				Type:   "error",
				Error:  llmErrorMessage("LLM request failed", err),
				Status: llmErrorStatus(err),
			})
			return
		}

		operations, err := extractEditorAssistantOperations(toolResults)
		if err != nil {
			_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
				Type:   "error",
				Error:  err.Error(),
				Status: fiber.StatusInternalServerError,
			})
			return
		}

		_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
			Type: "done",
			Response: editorAssistantResponsePayload{
				Content:     resp.Content,
				ToolCalls:   toolCalls,
				ToolResults: toolResults,
				Usage:       resp.Usage,
				Operations:  operations,
			},
		})
	})

	return nil
}

func (h *EditorAssistantHandler) resolveProvider(ctx context.Context, providerID string) (*models.LLMProvider, llm.Config, error) {
	if trimmed := strings.TrimSpace(providerID); trimmed != "" {
		provider, err := h.providerStore.GetByID(ctx, trimmed)
		if err != nil {
			return nil, llm.Config{}, fmt.Errorf("provider not found")
		}
		config, err := llm.ConfigFromModel(provider)
		if err != nil {
			return nil, llm.Config{}, fmt.Errorf("invalid provider configuration: %w", err)
		}
		return provider, config, nil
	}

	provider, err := h.providerStore.GetDefault(ctx)
	if err != nil {
		return nil, llm.Config{}, fmt.Errorf("no default LLM provider configured")
	}
	config, err := llm.ConfigFromModel(provider)
	if err != nil {
		return nil, llm.Config{}, fmt.Errorf("invalid provider configuration: %w", err)
	}
	return provider, config, nil
}

func (h *EditorAssistantHandler) buildModelMessages(
	profile assistants.Profile,
	mode string,
	history []editorAssistantHistoryMessage,
	message string,
	pipelineSnapshot assistants.PipelineSnapshot,
	selection assistants.SelectionSnapshot,
) ([]llm.Message, error) {
	modelMessages := []llm.Message{
		{
			Role:    "system",
			Content: h.systemPrompt(profile, mode),
		},
		{
			Role:    "system",
			Content: buildEditorAssistantContextMessage(pipelineSnapshot, selection),
		},
	}

	for _, item := range history {
		role := strings.TrimSpace(strings.ToLower(item.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		modelMessages = append(modelMessages, llm.Message{
			Role:    role,
			Content: content,
		})
	}

	modelMessages = append(modelMessages, llm.Message{
		Role:    "user",
		Content: message,
	})

	return modelMessages, nil
}

func (h *EditorAssistantHandler) systemPrompt(profile assistants.Profile, mode string) string {
	modeSection := strings.TrimSpace(fmt.Sprintf(`
Current mode: %s

Mode contract:
- Ask mode is read-only. You may explain the current pipeline, answer questions, and suggest changes, but you must not perform graph edits.
- Edit mode may change the live browser pipeline only by calling apply_live_pipeline_edits.
- The browser-provided pipeline snapshot is the source of truth and may include unsaved changes not present in the database.
- Never claim that graph edits were applied unless the live edit tool succeeds.
- If the user asks for something that requires the other mode, explicitly tell them to switch modes.
- Prefer minimal, precise changes and preserve existing node and edge ids whenever possible.
- Never write to the database or talk as if save already happened. The user chooses when to save the pipeline.
`, mode))

	sections := make([]string, 0, 2)
	if base := assistants.BuildPromptAppendix(profile); base != "" {
		sections = append(sections, base)
	}
	sections = append(sections, modeSection)

	if mode == "edit" {
		sections = append(sections, strings.TrimSpace(`
In edit mode:
- Use apply_live_pipeline_edits when a user asks to add, remove, reconnect, or modify graph elements.
- Send complete replacement node or edge objects for update operations.
- Preserve unrelated nodes, edges, and fields.
`))
	}

	return strings.Join(sections, "\n\n")
}

func buildEditorAssistantContextMessage(
	pipelineSnapshot assistants.PipelineSnapshot,
	selection assistants.SelectionSnapshot,
) string {
	type contextEnvelope struct {
		Pipeline  assistants.PipelineSnapshot  `json:"pipeline"`
		Selection assistants.SelectionSnapshot `json:"selection"`
	}

	payload, err := json.MarshalIndent(contextEnvelope{
		Pipeline:  pipelineSnapshot,
		Selection: selection,
	}, "", "  ")
	if err != nil {
		return "Current editor context is available, but failed to serialize it."
	}

	return strings.TrimSpace(`
Current node editor context from the browser:
- This snapshot is the source of truth for the current request.
- It may contain unsaved edits that are not in persistent storage.

` + string(payload))
}

func newEditorAssistantToolExecutor(snapshot assistants.PipelineSnapshot, enabled bool) *editorAssistantToolExecutor {
	return &editorAssistantToolExecutor{
		snapshot: snapshot,
		enabled:  enabled,
	}
}

func (e *editorAssistantToolExecutor) GetAllTools() []llm.ToolDefinition {
	if !e.enabled {
		return nil
	}

	return []llm.ToolDefinition{
		{
			Type: "function",
			Function: llm.ToolSpec{
				Name:        "apply_live_pipeline_edits",
				Description: "Validate and normalize live pipeline edit operations against the current browser pipeline snapshot. This updates only the in-memory editor state and never saves to the database.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"operations": map[string]interface{}{
							"type":        "array",
							"description": "Ordered live edit operations to apply to the current pipeline snapshot.",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"type": map[string]interface{}{
										"type":        "string",
										"description": "One of add_nodes, update_nodes, delete_nodes, add_edges, update_edges, delete_edges, set_viewport.",
									},
									"nodes": map[string]interface{}{
										"type":        "array",
										"description": "Full node objects for add_nodes or update_nodes.",
										"items":       map[string]interface{}{"type": "object"},
									},
									"edges": map[string]interface{}{
										"type":        "array",
										"description": "Full edge objects for add_edges or update_edges.",
										"items":       map[string]interface{}{"type": "object"},
									},
									"node_ids": map[string]interface{}{
										"type":        "array",
										"description": "Node ids to remove for delete_nodes.",
										"items":       map[string]interface{}{"type": "string"},
									},
									"edge_ids": map[string]interface{}{
										"type":        "array",
										"description": "Edge ids to remove for delete_edges.",
										"items":       map[string]interface{}{"type": "string"},
									},
									"viewport": map[string]interface{}{
										"type":        "object",
										"description": "Viewport object with x, y, and zoom for set_viewport.",
									},
								},
								"required": []string{"type"},
							},
						},
					},
					"required": []string{"operations"},
				},
			},
		},
	}
}

func (e *editorAssistantToolExecutor) Execute(ctx context.Context, name string, arguments json.RawMessage) (any, error) {
	if !e.enabled || name != "apply_live_pipeline_edits" {
		return nil, fmt.Errorf("tool %q is not available", name)
	}

	var req struct {
		Operations []assistants.LivePipelineOperation `json:"operations"`
	}
	if err := json.Unmarshal(arguments, &req); err != nil {
		return nil, fmt.Errorf("parse live edit arguments: %w", err)
	}

	normalized, nextSnapshot, err := assistants.ValidateAndApplyOperations(e.snapshot, req.Operations)
	if err != nil {
		return nil, err
	}

	e.snapshot = nextSnapshot

	return map[string]any{
		"operations": normalized,
	}, nil
}

func extractEditorAssistantOperations(toolResults []llm.ToolResult) ([]assistants.LivePipelineOperation, error) {
	operations := make([]assistants.LivePipelineOperation, 0)
	for _, toolResult := range toolResults {
		if toolResult.Tool != "apply_live_pipeline_edits" || toolResult.Result == nil {
			continue
		}

		var payload struct {
			Operations []assistants.LivePipelineOperation `json:"operations"`
		}
		encoded, err := json.Marshal(toolResult.Result)
		if err != nil {
			return nil, fmt.Errorf("encode live edit result: %w", err)
		}
		if err := json.Unmarshal(encoded, &payload); err != nil {
			return nil, fmt.Errorf("decode live edit result: %w", err)
		}
		operations = append(operations, payload.Operations...)
	}

	if len(operations) == 0 {
		return nil, nil
	}

	return operations, nil
}
