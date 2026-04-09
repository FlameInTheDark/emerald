package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FlameInTheDark/automator/internal/db/models"
	"github.com/FlameInTheDark/automator/internal/llm"
)

const (
	contextCompactionThreshold = 0.80
	contextCompactionPasses    = 3
	contextRawMessageBuffer    = 8
	compactionSummaryMaxTokens = 700
	compactionMessageLimit     = 1600
)

func (h *LLMChatHandler) prepareConversationModelMessages(
	ctx context.Context,
	provider llm.Provider,
	providerConfig llm.Config,
	systemPrompt string,
	conversation *models.ChatConversation,
	storedMessages []models.ChatMessage,
	pendingUserMessage string,
) ([]llm.Message, error) {
	if conversation == nil {
		return []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: pendingUserMessage},
		}, nil
	}

	conversation.ContextWindow = llm.ResolveContextWindow(providerConfig)

	for pass := 0; pass < contextCompactionPasses; pass++ {
		modelMessages, err := buildConversationModelMessages(systemPrompt, conversation, storedMessages, pendingUserMessage)
		if err != nil {
			return nil, err
		}
		if !shouldCompactConversation(conversation.ContextWindow, llm.EstimateMessagesTokens(modelMessages), len(storedMessages), conversation.CompactedMessageCount) {
			return modelMessages, nil
		}

		compacted, err := h.compactConversationContext(ctx, provider, providerConfig.Model, conversation, storedMessages)
		if err != nil {
			return nil, err
		}
		if !compacted {
			return modelMessages, nil
		}
	}

	return buildConversationModelMessages(systemPrompt, conversation, storedMessages, pendingUserMessage)
}

func (h *LLMChatHandler) compactConversationContext(
	ctx context.Context,
	provider llm.Provider,
	model string,
	conversation *models.ChatConversation,
	storedMessages []models.ChatMessage,
) (bool, error) {
	if conversation == nil {
		return false, nil
	}

	targetCount := len(storedMessages) - contextRawMessageBuffer
	if targetCount <= conversation.CompactedMessageCount {
		return false, nil
	}

	if delta := targetCount - conversation.CompactedMessageCount; delta%2 != 0 {
		targetCount--
	}
	if targetCount <= conversation.CompactedMessageCount {
		return false, nil
	}

	segment := storedMessages[conversation.CompactedMessageCount:targetCount]
	summary, err := h.summarizeConversationSegment(ctx, provider, model, conversation.ContextSummary, segment)
	if err != nil {
		return false, err
	}

	conversation.ContextSummary = stringPtr(summary)
	conversation.CompactedMessageCount = targetCount
	conversation.CompactionCount++
	now := time.Now().UTC()
	conversation.CompactedAt = &now

	if err := h.chatStore.UpdateConversationSettings(ctx, conversation); err != nil {
		return false, fmt.Errorf("persist compacted conversation context: %w", err)
	}

	return true, nil
}

func (h *LLMChatHandler) summarizeConversationSegment(
	ctx context.Context,
	provider llm.Provider,
	model string,
	existingSummary *string,
	messages []models.ChatMessage,
) (string, error) {
	transcript := formatCompactionTranscript(messages)
	if strings.TrimSpace(transcript) == "" {
		return derefString(existingSummary), nil
	}

	userPrompt := strings.TrimSpace(strings.Join([]string{
		"Update the hidden conversation memory for this chat.",
		formatExistingSummary(existingSummary),
		"New transcript to compact:",
		transcript,
		"Return only the updated hidden memory as concise bullet points. Preserve goals, constraints, environment facts, tool findings, decisions, and unfinished work.",
	}, "\n\n"))

	resp, err := provider.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "You write hidden conversation memory for a chat assistant. Keep it concise, factual, and reusable. Do not address the user directly.",
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		MaxTokens: compactionSummaryMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("compact conversation context: %w", err)
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", fmt.Errorf("compact conversation context: empty summary")
	}

	return summary, nil
}

func updateConversationContextStats(systemPrompt string, conversation *models.ChatConversation, storedMessages []models.ChatMessage) error {
	if conversation == nil {
		return nil
	}

	modelMessages, err := buildConversationModelMessages(systemPrompt, conversation, storedMessages, "")
	if err != nil {
		return err
	}

	conversation.ContextTokenCount = llm.EstimateMessagesTokens(modelMessages)
	return nil
}

func buildConversationModelMessages(
	systemPrompt string,
	conversation *models.ChatConversation,
	storedMessages []models.ChatMessage,
	pendingUserMessage string,
) ([]llm.Message, error) {
	modelMessages := make([]llm.Message, 0, len(storedMessages)+3)
	modelMessages = append(modelMessages, llm.Message{Role: "system", Content: systemPrompt})

	if conversation != nil {
		if summary := strings.TrimSpace(derefString(conversation.ContextSummary)); summary != "" {
			modelMessages = append(modelMessages, llm.Message{
				Role:    "system",
				Content: "Hidden conversation memory from earlier turns:\n" + summary,
			})
		}
	}

	history, err := buildModelHistory(storedMessages, compactedMessageOffset(conversation))
	if err != nil {
		return nil, err
	}
	modelMessages = append(modelMessages, history...)

	if trimmed := strings.TrimSpace(pendingUserMessage); trimmed != "" {
		modelMessages = append(modelMessages, llm.Message{Role: "user", Content: trimmed})
	}

	return modelMessages, nil
}

func buildModelHistory(messages []models.ChatMessage, compactedMessageCount int) ([]llm.Message, error) {
	start := compactedMessageCount
	if start < 0 {
		start = 0
	}
	if start > len(messages) {
		start = len(messages)
	}

	result := make([]llm.Message, 0, len(messages)-start)
	for _, message := range messages[start:] {
		replayMessages, err := buildStoredMessageReplay(message)
		if err != nil {
			return nil, err
		}
		result = append(result, replayMessages...)
	}
	return result, nil
}

func buildStoredMessageReplay(message models.ChatMessage) ([]llm.Message, error) {
	if message.ContextMessages != nil && strings.TrimSpace(*message.ContextMessages) != "" {
		var replayMessages []llm.Message
		if err := json.Unmarshal([]byte(*message.ContextMessages), &replayMessages); err != nil {
			return nil, fmt.Errorf("decode context messages: %w", err)
		}
		if len(replayMessages) > 0 {
			return replayMessages, nil
		}
	}

	replayMessage := llm.Message{
		Role:    message.Role,
		Content: message.Content,
	}
	if message.ToolCalls != nil && strings.TrimSpace(*message.ToolCalls) != "" {
		var toolCalls []llm.ToolCall
		if err := json.Unmarshal([]byte(*message.ToolCalls), &toolCalls); err == nil && len(toolCalls) > 0 {
			replayMessage.ToolCalls = toolCalls
		}
	}

	return []llm.Message{replayMessage}, nil
}

func shouldCompactConversation(contextWindow int, estimatedTokens int, messageCount int, compactedMessageCount int) bool {
	if contextWindow <= 0 {
		return false
	}
	if messageCount-compactedMessageCount <= contextRawMessageBuffer {
		return false
	}
	threshold := int(float64(contextWindow) * contextCompactionThreshold)
	return estimatedTokens >= threshold
}

func compactedMessageOffset(conversation *models.ChatConversation) int {
	if conversation == nil {
		return 0
	}
	return conversation.CompactedMessageCount
}

func formatExistingSummary(summary *string) string {
	if strings.TrimSpace(derefString(summary)) == "" {
		return "Existing hidden memory:\n(none)"
	}
	return "Existing hidden memory:\n" + strings.TrimSpace(derefString(summary))
}

func formatCompactionTranscript(messages []models.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, message := range messages {
		builder.WriteString(strings.ToUpper(strings.TrimSpace(message.Role)))
		builder.WriteString(": ")
		builder.WriteString(truncateCompactionText(message.Content))
		builder.WriteByte('\n')

		if message.ToolResults != nil && strings.TrimSpace(*message.ToolResults) != "" {
			builder.WriteString("TOOL RESULTS: ")
			builder.WriteString(truncateCompactionText(*message.ToolResults))
			builder.WriteByte('\n')
		}
	}

	return strings.TrimSpace(builder.String())
}

func truncateCompactionText(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return "(empty)"
	}

	runes := []rune(normalized)
	if len(runes) <= compactionMessageLimit {
		return normalized
	}
	return string(runes[:compactionMessageLimit-1]) + "…"
}
