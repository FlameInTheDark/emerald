package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/assistants"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/skills"
)

type EditorAssistantHandler struct {
	providerStore     *query.LLMProviderStore
	assistantProfiles *assistants.Store
	skillStore        skills.Reader
	validator         assistants.FlowValidator
}

type editorAssistantRequest struct {
	ProviderID  string                             `json:"provider_id,omitempty"`
	Mode        string                             `json:"mode"`
	Message     string                             `json:"message"`
	Messages    []editorAssistantHistoryMessage    `json:"messages"`
	Pipeline    assistants.PipelineSnapshot        `json:"pipeline"`
	Selection   assistants.SelectionSnapshot       `json:"selection"`
	AttachedLog *assistants.ExecutionLogAttachment `json:"attached_log,omitempty"`
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
	snapshot    assistants.PipelineSnapshot
	editEnabled bool
	skillStore  skills.Reader
	validator   assistants.FlowValidator
}

func NewEditorAssistantHandler(
	providerStore *query.LLMProviderStore,
	assistantProfiles *assistants.Store,
	skillStore skills.Reader,
	validators ...assistants.FlowValidator,
) *EditorAssistantHandler {
	handler := &EditorAssistantHandler{
		providerStore:     providerStore,
		assistantProfiles: assistantProfiles,
		skillStore:        skillStore,
	}
	if len(validators) > 0 {
		handler.validator = validators[0]
	}
	return handler
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

	modelMessages, err := h.buildModelMessages(profile, mode, req.Messages, req.Message, snapshot, req.Selection, req.AttachedLog)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	streamCtx := context.Background()
	if userCtx := c.UserContext(); userCtx != nil {
		streamCtx = userCtx
	}

	toolExecutor := newEditorAssistantToolExecutor(snapshot, mode == "edit", h.skillStore, h.validator)

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
	attachedLog *assistants.ExecutionLogAttachment,
) ([]llm.Message, error) {
	modelMessages := []llm.Message{
		{
			Role:    "system",
			Content: h.systemPrompt(profile, mode),
		},
		{
			Role:    "system",
			Content: buildEditorAssistantContextMessage(pipelineSnapshot, selection, attachedLog),
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

	sections := make([]string, 0, 4)
	if base := assistants.BuildPromptAppendix(profile); base != "" {
		sections = append(sections, base)
	}
	if skillGuidance := h.skillGuidance(profile); skillGuidance != "" {
		sections = append(sections, skillGuidance)
	}
	sections = append(sections, modeSection)

	if mode == "edit" {
		sections = append(sections, strings.TrimSpace(`
In edit mode:
- Use apply_live_pipeline_edits when a user asks to add, remove, reconnect, or modify graph elements.
- For update operations, send the node or edge id plus only the fields that should change.
- Preserve unrelated nodes, edges, and fields.
`))
	}

	return strings.Join(sections, "\n\n")
}

func (h *EditorAssistantHandler) skillGuidance(profile assistants.Profile) string {
	if h == nil || h.skillStore == nil {
		return ""
	}

	summaryByName := make(map[string]skills.Summary)
	for _, item := range h.skillStore.List() {
		summaryByName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}

	names := assistants.SkillNamesForModules(profile.EnabledModules)
	if _, ok := summaryByName["pipeline-builder"]; ok {
		names = append(names, "pipeline-builder")
	}

	lines := make([]string, 0, len(names)+2)
	seen := make(map[string]struct{}, len(names))
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		line := "- " + name
		if summary, ok := summaryByName[key]; ok && strings.TrimSpace(summary.Description) != "" {
			line += ": " + strings.TrimSpace(summary.Description)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join([]string{
		"Read local skills on demand with get_skill instead of guessing details.",
		"Relevant local skills:",
		strings.Join(lines, "\n"),
		"Use get_skill only for the skill that matches the current question or requested edit.",
	}, "\n")
}

func buildEditorAssistantContextMessage(
	pipelineSnapshot assistants.PipelineSnapshot,
	selection assistants.SelectionSnapshot,
	attachedLog *assistants.ExecutionLogAttachment,
) string {
	type contextEnvelope struct {
		Pipeline    assistants.PipelineSnapshot        `json:"pipeline"`
		Selection   assistants.SelectionSnapshot       `json:"selection"`
		AttachedLog *assistants.ExecutionLogAttachment `json:"attached_log,omitempty"`
	}

	payload, err := json.MarshalIndent(contextEnvelope{
		Pipeline:    pipelineSnapshot,
		Selection:   selection,
		AttachedLog: attachedLog,
	}, "", "  ")
	if err != nil {
		return "Current editor context is available, but failed to serialize it."
	}

	lines := []string{
		"Current node editor context from the browser:",
		"- This snapshot is the source of truth for the current request.",
		"- It may contain unsaved edits that are not in persistent storage.",
	}
	if attachedLog != nil {
		lines = append(lines,
			"- attached_log is one real execution sample the user selected to share with you.",
			"- Use it as runtime reference data for what actually flowed through the pipeline in one run.",
		)
	}

	return strings.TrimSpace(strings.Join(lines, "\n") + `

` + string(payload))
}

func newEditorAssistantToolExecutor(snapshot assistants.PipelineSnapshot, editEnabled bool, skillStore skills.Reader, validators ...assistants.FlowValidator) *editorAssistantToolExecutor {
	executor := &editorAssistantToolExecutor{
		snapshot:    snapshot,
		editEnabled: editEnabled,
		skillStore:  skillStore,
	}
	if len(validators) > 0 {
		executor.validator = validators[0]
	}
	return executor
}

func (e *editorAssistantToolExecutor) GetAllTools() []llm.ToolDefinition {
	tools := make([]llm.ToolDefinition, 0, 2)
	if e.skillStore != nil {
		tools = append(tools, llm.SkillToolDefinition())
	}
	if !e.editEnabled {
		return tools
	}

	tools = append(tools, llm.ToolDefinition{
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
									"description": "For add_nodes, provide full React Flow node objects with id, position, and data. For update_nodes, provide node id plus only the fields that should change; existing fields are preserved.",
									"items":       map[string]interface{}{"type": "object"},
								},
								"edges": map[string]interface{}{
									"type":        "array",
									"description": "For add_edges, provide full edge objects with id, source, and target. For update_edges, provide edge id plus only the fields that should change; existing fields are preserved.",
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
	})

	return tools
}

func (e *editorAssistantToolExecutor) Execute(ctx context.Context, name string, arguments json.RawMessage) (any, error) {
	if name == "get_skill" && e.skillStore != nil {
		return llm.ExecuteSkillTool(ctx, e.skillStore, arguments)
	}

	if !e.editEnabled || name != "apply_live_pipeline_edits" {
		return nil, fmt.Errorf("tool %q is not available", name)
	}

	var req struct {
		Operations []assistants.LivePipelineOperation `json:"operations"`
	}
	if err := json.Unmarshal(arguments, &req); err != nil {
		return nil, fmt.Errorf("parse live edit arguments: %w", err)
	}

	normalized, nextSnapshot, err := assistants.ValidateAndApplyOperations(e.snapshot, req.Operations, e.validator)
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
