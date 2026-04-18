package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/assistants"
	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/skills"
)

const authSessionLocalKey = "auth_session"
const llmChatTimeLayout = "2006-01-02T15:04:05Z07:00"

type LLMChatHandler struct {
	providerStore     *query.LLMProviderStore
	clusterStore      *query.ClusterStore
	kubernetesStore   *query.KubernetesClusterStore
	pipelineStore     *query.PipelineStore
	chatStore         *query.ChatStore
	runner            *pipeline.ExecutionRunner
	scheduler         llm.ToolPipelineReloader
	skillStore        skills.Reader
	shellRunner       shellcmd.Runner
	assistantProfiles *assistants.Store
}

type llmChatRequest struct {
	ConversationID  string  `json:"conversation_id,omitempty"`
	Message         string  `json:"message"`
	ProviderID      string  `json:"provider_id,omitempty"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	ClusterID       string  `json:"cluster_id,omitempty"`
	Integrations    struct {
		Proxmox struct {
			Enabled   *bool  `json:"enabled,omitempty"`
			ClusterID string `json:"cluster_id,omitempty"`
		} `json:"proxmox"`
		Kubernetes struct {
			Enabled   *bool  `json:"enabled,omitempty"`
			ClusterID string `json:"cluster_id,omitempty"`
		} `json:"kubernetes"`
	} `json:"integrations"`
}

type llmConversationUpdateRequest struct {
	ProviderID      string  `json:"provider_id,omitempty"`
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`
	Integrations    struct {
		Proxmox struct {
			Enabled   *bool  `json:"enabled,omitempty"`
			ClusterID string `json:"cluster_id,omitempty"`
		} `json:"proxmox"`
		Kubernetes struct {
			Enabled   *bool  `json:"enabled,omitempty"`
			ClusterID string `json:"cluster_id,omitempty"`
		} `json:"kubernetes"`
	} `json:"integrations"`
}

type conversationSettings struct {
	ProviderID          *string
	ReasoningEffort     *string
	ProxmoxEnabled      bool
	ProxmoxClusterID    *string
	KubernetesEnabled   bool
	KubernetesClusterID *string
}

type conversationResponse struct {
	ID                   string                        `json:"id"`
	Title                string                        `json:"title"`
	ProviderID           *string                       `json:"provider_id,omitempty"`
	ReasoningEffort      *string                       `json:"reasoning_effort,omitempty"`
	ProxmoxEnabled       bool                          `json:"proxmox_enabled"`
	ProxmoxClusterID     *string                       `json:"proxmox_cluster_id,omitempty"`
	KubernetesEnabled    bool                          `json:"kubernetes_enabled"`
	KubernetesClusterID  *string                       `json:"kubernetes_cluster_id,omitempty"`
	CompactionCount      int                           `json:"compaction_count"`
	CompactedAt          *string                       `json:"compacted_at,omitempty"`
	ContextWindow        int                           `json:"context_window"`
	ContextTokenCount    int                           `json:"context_token_count"`
	LastPromptTokens     int                           `json:"last_prompt_tokens"`
	LastCompletionTokens int                           `json:"last_completion_tokens"`
	LastTotalTokens      int                           `json:"last_total_tokens"`
	LastMessageAt        string                        `json:"last_message_at"`
	CreatedAt            string                        `json:"created_at"`
	UpdatedAt            string                        `json:"updated_at"`
	Messages             []conversationMessageResponse `json:"messages,omitempty"`
}

type conversationMessageResponse struct {
	ID              string           `json:"id"`
	Role            string           `json:"role"`
	Content         string           `json:"content"`
	Reasoning       string           `json:"reasoning,omitempty"`
	ToolCalls       []llm.ToolCall   `json:"tool_calls,omitempty"`
	ToolResults     []llm.ToolResult `json:"tool_results,omitempty"`
	ContextMessages []llm.Message    `json:"context_messages,omitempty"`
	Usage           *llm.Usage       `json:"usage,omitempty"`
	CreatedAt       string           `json:"created_at"`
}

func NewLLMChatHandler(
	providerStore *query.LLMProviderStore,
	clusterStore *query.ClusterStore,
	kubernetesStore *query.KubernetesClusterStore,
	pipelineStore *query.PipelineStore,
	chatStore *query.ChatStore,
	runner *pipeline.ExecutionRunner,
	scheduler llm.ToolPipelineReloader,
	skillStore skills.Reader,
	shellRunner shellcmd.Runner,
	assistantProfiles *assistants.Store,
) *LLMChatHandler {
	return &LLMChatHandler{
		providerStore:     providerStore,
		clusterStore:      clusterStore,
		kubernetesStore:   kubernetesStore,
		pipelineStore:     pipelineStore,
		chatStore:         chatStore,
		runner:            runner,
		scheduler:         scheduler,
		skillStore:        skillStore,
		shellRunner:       shellRunner,
		assistantProfiles: assistantProfiles,
	}
}

func (h *LLMChatHandler) ListConversations(c *fiber.Ctx) error {
	session, ok := currentAuthSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	conversations, err := h.chatStore.ListByUser(c.Context(), session.UserID)
	if err != nil {
		status := llmErrorStatus(err)
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}
	if conversations == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	responses := make([]conversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		responses = append(responses, newConversationResponse(conversation, nil))
	}
	return c.JSON(responses)
}

func (h *LLMChatHandler) GetConversation(c *fiber.Ctx) error {
	session, ok := currentAuthSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	conversation, err := h.chatStore.GetConversationByID(c.Context(), session.UserID, c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if conversation == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	}

	messages, err := h.chatStore.ListMessages(c.Context(), conversation.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	responseMessages, err := decodeConversationMessages(messages)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newConversationResponse(*conversation, responseMessages))
}

func (h *LLMChatHandler) UpdateConversation(c *fiber.Ctx) error {
	session, ok := currentAuthSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	conversation, err := h.chatStore.GetConversationByID(c.Context(), session.UserID, c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if conversation == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	}

	var req llmConversationUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	settings := settingsFromConversation(*conversation)
	applySettingsOverrides(&settings, req.ProviderID, req.ReasoningEffort, "", req.Integrations.Proxmox.Enabled, req.Integrations.Proxmox.ClusterID, req.Integrations.Kubernetes.Enabled, req.Integrations.Kubernetes.ClusterID)
	applySettingsToConversation(conversation, settings)

	if err := h.chatStore.UpdateConversationSettings(c.Context(), conversation); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	reloaded, err := h.chatStore.GetConversationByID(c.Context(), session.UserID, conversation.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if reloaded == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
	}

	return c.JSON(newConversationResponse(*reloaded, nil))
}

func (h *LLMChatHandler) DeleteConversation(c *fiber.Ctx) error {
	session, ok := currentAuthSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	if err := h.chatStore.DeleteConversation(c.Context(), session.UserID, c.Params("id")); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *LLMChatHandler) Chat(c *fiber.Ctx) error {
	session, ok := currentAuthSession(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	var req llmChatRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "message is required"})
	}

	prepared, err := h.prepareChatTurn(c.Context(), session, req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
		}
		if errors.Is(err, errProviderNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	modelMessages, err := h.prepareConversationModelMessages(
		c.Context(),
		prepared.provider,
		prepared.providerConfig,
		prepared.contextWindow,
		prepared.systemPrompt,
		prepared.conversation,
		prepared.storedMessages,
		req.Message,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	resp, toolCalls, toolResults, contextMessages, err := runToolChat(
		c.Context(),
		prepared.provider,
		prepared.providerConfig.Model,
		modelMessages,
		prepared.toolRegistry,
		derefString(prepared.conversation.ReasoningEffort),
	)
	if err != nil {
		status := llmErrorStatus(err)
		return c.Status(status).JSON(fiber.Map{"error": llmErrorMessage("LLM request failed", err)})
	}

	reloaded, err := h.persistChatTurn(c.Context(), prepared, req.Message, resp, toolCalls, toolResults, contextMessages)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "conversation not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"conversation_id":  prepared.conversation.ID,
		"conversation":     newConversationResponse(*reloaded, nil),
		"content":          resp.Content,
		"reasoning":        resp.Reasoning,
		"tool_calls":       toolCalls,
		"tool_results":     toolResults,
		"context_messages": contextMessages,
		"usage":            resp.Usage,
	})
}

func currentAuthSession(c *fiber.Ctx) (auth.Session, bool) {
	session, ok := c.Locals(authSessionLocalKey).(auth.Session)
	return session, ok
}

var errProviderNotFound = errors.New("provider not found")

func (h *LLMChatHandler) resolveProvider(ctx context.Context, providerID *string) (*models.LLMProvider, llm.Config, error) {
	if providerID != nil && strings.TrimSpace(*providerID) != "" {
		provider, err := h.providerStore.GetByID(ctx, strings.TrimSpace(*providerID))
		if err != nil {
			return nil, llm.Config{}, errProviderNotFound
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

func (h *LLMChatHandler) loadProxmoxCluster(ctx context.Context, enabled bool, clusterID *string) (*models.Cluster, error) {
	if !enabled || clusterID == nil || strings.TrimSpace(*clusterID) == "" {
		return nil, nil
	}
	cluster, err := h.clusterStore.GetByID(ctx, strings.TrimSpace(*clusterID))
	if err != nil {
		return nil, fmt.Errorf("cluster not found")
	}
	return cluster, nil
}

func (h *LLMChatHandler) loadKubernetesCluster(ctx context.Context, enabled bool, clusterID *string) (*models.KubernetesCluster, error) {
	if !enabled || clusterID == nil || strings.TrimSpace(*clusterID) == "" {
		return nil, nil
	}
	cluster, err := h.kubernetesStore.GetByID(ctx, strings.TrimSpace(*clusterID))
	if err != nil {
		return nil, fmt.Errorf("kubernetes cluster not found")
	}
	return cluster, nil
}

func (h *LLMChatHandler) systemPrompt(
	profile assistants.Profile,
	selectedProxmoxCluster *models.Cluster,
	selectedKubernetesCluster *models.KubernetesCluster,
	settings conversationSettings,
) string {
	systemPrompt := assistants.BuildPromptAppendix(profile)
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = assistants.DefaultProfile(assistants.ScopeChatWindow).SystemInstructions
	}
	integrationStatements := make([]string, 0, 2)
	if settings.ProxmoxEnabled {
		if selectedProxmoxCluster != nil {
			integrationStatements = append(integrationStatements, "Proxmox integration is enabled and the selected cluster is "+selectedProxmoxCluster.Name+" ("+selectedProxmoxCluster.ID+"). Use it for Proxmox tool calls unless the user explicitly asks for a different configured cluster.")
		} else {
			integrationStatements = append(integrationStatements, "Proxmox integration is enabled.")
		}
	}
	if settings.KubernetesEnabled {
		if selectedKubernetesCluster != nil {
			integrationStatements = append(integrationStatements, "Kubernetes integration is enabled and the selected cluster is "+selectedKubernetesCluster.Name+" ("+selectedKubernetesCluster.ID+") using context "+selectedKubernetesCluster.ContextName+". Use it for Kubernetes tool calls unless the user explicitly asks for a different configured cluster.")
		} else {
			integrationStatements = append(integrationStatements, "Kubernetes integration is enabled.")
		}
	}
	if len(integrationStatements) == 0 {
		systemPrompt += " No infrastructure integration is enabled for this chat, so only local, pipeline, and skill tools are available."
	} else {
		systemPrompt += " " + strings.Join(integrationStatements, " ")
	}
	if h.skillStore != nil {
		if skillsSummary := h.skillStore.SummaryText(); skillsSummary != "" {
			systemPrompt += " Local skills are available:\n" + skillsSummary + "\nUse the get_skill tool when you need the full contents of one."
			systemPrompt += " For pipeline creation or structural edits, read the pipeline-builder skill before building a new definition."
		}
	}
	if h.shellRunner != nil {
		systemPrompt += " A run_shell_command tool is available when you need to inspect or operate in the local workspace."
	}
	return systemPrompt
}

func defaultConversationSettings() conversationSettings {
	return conversationSettings{
		ProxmoxEnabled:    true,
		KubernetesEnabled: false,
	}
}

func settingsFromConversation(conversation models.ChatConversation) conversationSettings {
	return conversationSettings{
		ProviderID:          conversation.ProviderID,
		ReasoningEffort:     conversation.ReasoningEffort,
		ProxmoxEnabled:      conversation.ProxmoxEnabled,
		ProxmoxClusterID:    conversation.ProxmoxClusterID,
		KubernetesEnabled:   conversation.KubernetesEnabled,
		KubernetesClusterID: conversation.KubernetesClusterID,
	}
}

func applySettingsOverrides(
	settings *conversationSettings,
	providerID string,
	reasoningEffort *string,
	legacyClusterID string,
	proxmoxEnabled *bool,
	proxmoxClusterID string,
	kubernetesEnabled *bool,
	kubernetesClusterID string,
) {
	if settings == nil {
		return
	}

	if trimmed := strings.TrimSpace(providerID); trimmed != "" {
		settings.ProviderID = stringPtr(trimmed)
	}
	if reasoningEffort != nil {
		settings.ReasoningEffort = stringPtr(strings.TrimSpace(*reasoningEffort))
	}

	if proxmoxEnabled != nil {
		settings.ProxmoxEnabled = *proxmoxEnabled
	}
	if trimmed := strings.TrimSpace(proxmoxClusterID); trimmed != "" {
		settings.ProxmoxEnabled = true
		settings.ProxmoxClusterID = stringPtr(trimmed)
	} else if trimmed := strings.TrimSpace(legacyClusterID); trimmed != "" && proxmoxEnabled == nil {
		settings.ProxmoxEnabled = true
		settings.ProxmoxClusterID = stringPtr(trimmed)
	}
	if proxmoxEnabled != nil && !*proxmoxEnabled && strings.TrimSpace(proxmoxClusterID) == "" {
		settings.ProxmoxClusterID = nil
	}

	if kubernetesEnabled != nil {
		settings.KubernetesEnabled = *kubernetesEnabled
	}
	if trimmed := strings.TrimSpace(kubernetesClusterID); trimmed != "" {
		settings.KubernetesEnabled = true
		settings.KubernetesClusterID = stringPtr(trimmed)
	}
	if kubernetesEnabled != nil && !*kubernetesEnabled && strings.TrimSpace(kubernetesClusterID) == "" {
		settings.KubernetesClusterID = nil
	}
}

func applySettingsToConversation(conversation *models.ChatConversation, settings conversationSettings) {
	conversation.ProviderID = settings.ProviderID
	conversation.ReasoningEffort = settings.ReasoningEffort
	conversation.ProxmoxEnabled = settings.ProxmoxEnabled
	conversation.ProxmoxClusterID = settings.ProxmoxClusterID
	conversation.KubernetesEnabled = settings.KubernetesEnabled
	conversation.KubernetesClusterID = settings.KubernetesClusterID
}

func titleFromMessage(message string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if normalized == "" {
		return "New Conversation"
	}

	const maxLength = 72
	runes := []rune(normalized)
	if len(runes) <= maxLength {
		return normalized
	}
	return string(runes[:maxLength-1]) + "…"
}

func marshalJSONString(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}

	switch typed := value.(type) {
	case []llm.ToolCall:
		if len(typed) == 0 {
			return nil, nil
		}
	case []llm.ToolResult:
		if len(typed) == 0 {
			return nil, nil
		}
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json payload: %w", err)
	}
	if string(encoded) == "null" {
		return nil, nil
	}
	return stringPtr(string(encoded)), nil
}

func decodeConversationMessages(messages []models.ChatMessage) ([]conversationMessageResponse, error) {
	responses := make([]conversationMessageResponse, 0, len(messages))
	for _, message := range messages {
		response := conversationMessageResponse{
			ID:        message.ID,
			Role:      message.Role,
			Content:   message.Content,
			CreatedAt: message.CreatedAt.Format(llmChatTimeLayout),
		}
		if message.ToolCalls != nil {
			if err := json.Unmarshal([]byte(*message.ToolCalls), &response.ToolCalls); err != nil {
				return nil, fmt.Errorf("decode tool calls: %w", err)
			}
		}
		if message.ToolResults != nil {
			if err := json.Unmarshal([]byte(*message.ToolResults), &response.ToolResults); err != nil {
				return nil, fmt.Errorf("decode tool results: %w", err)
			}
		}
		if message.Usage != nil {
			var usage llm.Usage
			if err := json.Unmarshal([]byte(*message.Usage), &usage); err != nil {
				return nil, fmt.Errorf("decode usage: %w", err)
			}
			response.Usage = &usage
		}
		if message.ContextMessages != nil && strings.TrimSpace(*message.ContextMessages) != "" {
			if err := json.Unmarshal([]byte(*message.ContextMessages), &response.ContextMessages); err != nil {
				return nil, fmt.Errorf("decode context messages: %w", err)
			}
			if len(response.ContextMessages) == 1 {
				response.Reasoning = strings.TrimSpace(response.ContextMessages[0].Reasoning)
			}
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func newConversationResponse(conversation models.ChatConversation, messages []conversationMessageResponse) conversationResponse {
	var compactedAt *string
	if conversation.CompactedAt != nil {
		compactedAt = stringPtr(conversation.CompactedAt.Format(llmChatTimeLayout))
	}

	return conversationResponse{
		ID:                   conversation.ID,
		Title:                conversation.Title,
		ProviderID:           conversation.ProviderID,
		ReasoningEffort:      conversation.ReasoningEffort,
		ProxmoxEnabled:       conversation.ProxmoxEnabled,
		ProxmoxClusterID:     conversation.ProxmoxClusterID,
		KubernetesEnabled:    conversation.KubernetesEnabled,
		KubernetesClusterID:  conversation.KubernetesClusterID,
		CompactionCount:      conversation.CompactionCount,
		CompactedAt:          compactedAt,
		ContextWindow:        conversation.ContextWindow,
		ContextTokenCount:    conversation.ContextTokenCount,
		LastPromptTokens:     conversation.LastPromptTokens,
		LastCompletionTokens: conversation.LastCompletionTokens,
		LastTotalTokens:      conversation.LastTotalTokens,
		LastMessageAt:        conversation.LastMessageAt.Format(llmChatTimeLayout),
		CreatedAt:            conversation.CreatedAt.Format(llmChatTimeLayout),
		UpdatedAt:            conversation.UpdatedAt.Format(llmChatTimeLayout),
		Messages:             messages,
	}
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
