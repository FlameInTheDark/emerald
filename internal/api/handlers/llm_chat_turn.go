package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/automator/internal/assistants"
	"github.com/FlameInTheDark/automator/internal/auth"
	"github.com/FlameInTheDark/automator/internal/db/models"
	"github.com/FlameInTheDark/automator/internal/llm"
)

type preparedChatTurn struct {
	conversation       *models.ChatConversation
	createConversation bool
	providerConfig     llm.Config
	provider           llm.Provider
	toolRegistry       llm.ToolExecutor
	systemPrompt       string
	storedMessages     []models.ChatMessage
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
	applySettingsOverrides(&settings, req.ProviderID, req.ClusterID, req.Integrations.Proxmox.Enabled, req.Integrations.Proxmox.ClusterID, req.Integrations.Kubernetes.Enabled, req.Integrations.Kubernetes.ClusterID)

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
	conversation.ContextWindow = llm.ResolveContextWindow(prepared.providerConfig)
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
