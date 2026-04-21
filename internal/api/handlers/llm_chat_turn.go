package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/assistants"
	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/webtools"
)

type preparedChatTurn struct {
	conversation       *models.ChatConversation
	createConversation bool
	providerConfig     llm.Config
	contextWindow      int
	provider           llm.Provider
	toolRegistry       llm.ToolExecutor
	systemPrompt       string
	storedMessages     []models.ChatMessage
}

type streamingChatTurn struct {
	conversation       *models.ChatConversation
	createConversation bool
	storedMessages     []models.ChatMessage
	userMessage        *models.ChatMessage
	assistantMessage   *models.ChatMessage
}

type chatTurnSnapshot struct {
	response        *llm.ChatResponse
	toolCalls       []llm.ToolCall
	toolResults     []llm.ToolResult
	contextMessages []llm.Message
}

func (h *LLMChatHandler) prepareChatTurn(
	ctx context.Context,
	session auth.Session,
	req llmChatRequest,
) (*preparedChatTurn, error) {
	var conversation *models.ChatConversation
	createConversation := strings.TrimSpace(req.ConversationID) == ""
	if !createConversation {
		var err error
		conversation, err = h.chatStore.GetConversationByID(ctx, session.UserID, req.ConversationID)
		if err != nil {
			return nil, fmt.Errorf("load conversation: %w", err)
		}
		if conversation == nil {
			return nil, sql.ErrNoRows
		}
	}

	settings := defaultConversationSettings()
	if conversation != nil {
		settings = settingsFromConversation(*conversation)
	}
	applySettingsOverrides(&settings, req.ProviderID, req.ReasoningEffort, req.ClusterID, req.Integrations.Proxmox.Enabled, req.Integrations.Proxmox.ClusterID, req.Integrations.Kubernetes.Enabled, req.Integrations.Kubernetes.ClusterID)

	providerModel, providerConfig, err := h.resolveProvider(ctx, settings.ProviderID)
	if err != nil {
		return nil, err
	}
	settings.ProviderID = stringPtr(providerModel.ID)

	selectedProxmoxCluster, err := h.loadProxmoxCluster(ctx, settings.ProxmoxEnabled, settings.ProxmoxClusterID)
	if err != nil {
		return nil, err
	}
	selectedKubernetesCluster, err := h.loadKubernetesCluster(ctx, settings.KubernetesEnabled, settings.KubernetesClusterID)
	if err != nil {
		return nil, err
	}

	provider, err := llm.NewProvider(providerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	contextWindow := llm.ResolveContextWindowWithDiscovery(ctx, providerConfig)

	storedMessages := make([]models.ChatMessage, 0)
	if conversation != nil {
		storedMessages, err = h.chatStore.ListMessages(ctx, conversation.ID)
		if err != nil {
			return nil, fmt.Errorf("load stored messages: %w", err)
		}
	} else {
		conversation = &models.ChatConversation{
			UserID: session.UserID,
			Title:  titleFromMessage(req.Message),
		}
	}
	applySettingsToConversation(conversation, settings)

	var webToolsConfig *webtools.RuntimeConfig
	if h.webTools != nil {
		resolved, err := h.webTools.Resolve(ctx)
		if err != nil {
			return nil, fmt.Errorf("load web tools config: %w", err)
		}
		webToolsConfig = &resolved
	}

	toolRegistry := llm.NewToolRegistryWithOptions(llm.ToolRegistryOptions{
		ProxmoxStore:               h.clusterStore,
		KubernetesStore:            h.kubernetesStore,
		PipelineStore:              h.pipelineStore,
		PipelineReloader:           h.scheduler,
		PipelineRunner:             &chatPipelineRunner{store: h.pipelineStore, runner: h.runner},
		DefaultProxmoxClusterID:    derefString(settings.ProxmoxClusterID),
		DefaultKubernetesClusterID: derefString(settings.KubernetesClusterID),
		EnableProxmox:              settings.ProxmoxEnabled,
		EnableKubernetes:           settings.KubernetesEnabled,
		SkillStore:                 h.skillStore,
		ShellRunner:                h.shellRunner,
		WebToolsConfig:             webToolsConfig,
		AuditLogger:                h.toolAuditLogger(session),
	})

	profile, err := h.assistantProfiles.Get(ctx, assistants.ScopeChatWindow)
	if err != nil {
		return nil, fmt.Errorf("load chat assistant profile: %w", err)
	}

	systemPrompt := h.systemPrompt(profile, selectedProxmoxCluster, selectedKubernetesCluster, settings)

	return &preparedChatTurn{
		conversation:       conversation,
		createConversation: createConversation,
		providerConfig:     providerConfig,
		contextWindow:      contextWindow,
		provider:           provider,
		toolRegistry:       toolRegistry,
		systemPrompt:       systemPrompt,
		storedMessages:     storedMessages,
	}, nil
}

func (h *LLMChatHandler) persistChatTurn(
	ctx context.Context,
	prepared *preparedChatTurn,
	userMessage string,
	resp *llm.ChatResponse,
	toolCalls []llm.ToolCall,
	toolResults []llm.ToolResult,
	contextMessages []llm.Message,
) (*models.ChatConversation, error) {
	if prepared == nil || prepared.conversation == nil {
		return nil, fmt.Errorf("prepared chat turn is required")
	}

	toolCallsJSON, err := marshalJSONString(toolCalls)
	if err != nil {
		return nil, err
	}
	toolResultsJSON, err := marshalJSONString(toolResults)
	if err != nil {
		return nil, err
	}
	usageJSON, err := marshalJSONString(resp.Usage)
	if err != nil {
		return nil, err
	}
	contextMessagesJSON, err := marshalJSONString(contextMessages)
	if err != nil {
		return nil, err
	}

	conversation := prepared.conversation
	conversation.ContextWindow = prepared.contextWindow
	conversation.LastPromptTokens = resp.Usage.PromptTokens
	conversation.LastCompletionTokens = resp.Usage.CompletionTokens
	conversation.LastTotalTokens = resp.Usage.TotalTokens

	persistedMessages := append(append(make([]models.ChatMessage, 0, len(prepared.storedMessages)+2), prepared.storedMessages...),
		models.ChatMessage{Role: "user", Content: userMessage},
		models.ChatMessage{Role: "assistant", Content: resp.Content, ContextMessages: contextMessagesJSON},
	)
	if err := updateConversationContextStats(prepared.systemPrompt, conversation, persistedMessages); err != nil {
		return nil, err
	}

	if err := h.chatStore.AppendTurn(
		ctx,
		conversation,
		prepared.createConversation,
		&models.ChatMessage{Role: "user", Content: userMessage},
		&models.ChatMessage{
			Role:            "assistant",
			Content:         resp.Content,
			ToolCalls:       toolCallsJSON,
			ToolResults:     toolResultsJSON,
			Usage:           usageJSON,
			ContextMessages: contextMessagesJSON,
		},
	); err != nil {
		return nil, err
	}

	reloaded, err := h.chatStore.GetConversationByID(ctx, conversation.UserID, conversation.ID)
	if err != nil {
		return nil, err
	}
	if reloaded == nil {
		return nil, fmt.Errorf("conversation was not persisted")
	}

	return reloaded, nil
}

func (h *LLMChatHandler) beginStreamingChatTurn(
	ctx context.Context,
	prepared *preparedChatTurn,
	userMessage string,
) (*streamingChatTurn, error) {
	if prepared == nil || prepared.conversation == nil {
		return nil, fmt.Errorf("prepared chat turn is required")
	}

	turn := &streamingChatTurn{
		conversation:       prepared.conversation,
		createConversation: prepared.createConversation,
		storedMessages:     append([]models.ChatMessage(nil), prepared.storedMessages...),
		userMessage: &models.ChatMessage{
			Role:    "user",
			Content: userMessage,
		},
		assistantMessage: &models.ChatMessage{
			Role:    "assistant",
			Content: "",
		},
	}

	persistedMessages := append(append(make([]models.ChatMessage, 0, len(turn.storedMessages)+2), turn.storedMessages...),
		*turn.userMessage,
		*turn.assistantMessage,
	)
	if err := updateConversationContextStats(prepared.systemPrompt, turn.conversation, persistedMessages); err != nil {
		return nil, err
	}

	if err := h.chatStore.AppendTurn(
		ctx,
		turn.conversation,
		turn.createConversation,
		turn.userMessage,
		turn.assistantMessage,
	); err != nil {
		return nil, err
	}

	return turn, nil
}

func (h *LLMChatHandler) persistStreamingChatTurn(
	ctx context.Context,
	prepared *preparedChatTurn,
	turn *streamingChatTurn,
	snapshot chatTurnSnapshot,
) error {
	if prepared == nil || turn == nil || turn.conversation == nil || turn.userMessage == nil || turn.assistantMessage == nil {
		return fmt.Errorf("streaming chat turn is required")
	}

	toolCallsJSON, err := marshalJSONString(snapshot.toolCalls)
	if err != nil {
		return err
	}
	toolResultsJSON, err := marshalJSONString(snapshot.toolResults)
	if err != nil {
		return err
	}
	var usage llm.Usage
	if snapshot.response != nil {
		usage = snapshot.response.Usage
	}
	var usageJSON *string
	if usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		usageJSON, err = marshalJSONString(usage)
		if err != nil {
			return err
		}
	}
	contextMessagesJSON, err := marshalJSONString(snapshot.contextMessages)
	if err != nil {
		return err
	}

	turn.assistantMessage.Content = snapshotAssistantContent(snapshot.response, snapshot.contextMessages)
	turn.assistantMessage.ToolCalls = toolCallsJSON
	turn.assistantMessage.ToolResults = toolResultsJSON
	turn.assistantMessage.Usage = usageJSON
	turn.assistantMessage.ContextMessages = contextMessagesJSON

	turn.conversation.ContextWindow = prepared.contextWindow
	turn.conversation.LastPromptTokens = usage.PromptTokens
	turn.conversation.LastCompletionTokens = usage.CompletionTokens
	turn.conversation.LastTotalTokens = usage.TotalTokens

	persistedMessages := append(append(make([]models.ChatMessage, 0, len(turn.storedMessages)+2), turn.storedMessages...),
		*turn.userMessage,
		*turn.assistantMessage,
	)
	if err := updateConversationContextStats(prepared.systemPrompt, turn.conversation, persistedMessages); err != nil {
		return err
	}

	return h.chatStore.UpdateTurn(ctx, turn.conversation, turn.assistantMessage)
}

func snapshotAssistantContent(resp *llm.ChatResponse, contextMessages []llm.Message) string {
	if resp != nil && strings.TrimSpace(resp.Content) != "" {
		return resp.Content
	}

	parts := make([]string, 0, len(contextMessages))
	for _, message := range contextMessages {
		if strings.TrimSpace(message.Role) != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}

	return strings.Join(parts, "\n\n")
}
