package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DefaultMaxToolChatRounds    = 64
	toolChatContextThreshold    = 0.90
	toolChatContextReserveMin   = 2048
	toolChatContextReserveFrac  = 10
	toolChatCompactionPasses    = 3
	toolChatRawMessageBuffer    = 8
	toolChatSummaryMaxTokens    = 700
	toolChatSummaryMessageLimit = 1600
	toolChatSummaryPrefix       = "Hidden tool-run memory from earlier in this response:\n"
)

type ToolChatOptions struct {
	ReasoningEffort string
	MaxRounds       int
	ContextWindow   int
}

type ToolChatEventType string

const (
	ToolChatEventContentDelta   ToolChatEventType = "content_delta"
	ToolChatEventReasoningDelta ToolChatEventType = "reasoning_delta"
	ToolChatEventToolStarted    ToolChatEventType = "tool_started"
	ToolChatEventToolFinished   ToolChatEventType = "tool_finished"
	ToolChatEventUsage          ToolChatEventType = "usage"
)

type ToolChatEvent struct {
	Type       ToolChatEventType `json:"type"`
	Delta      string            `json:"delta,omitempty"`
	ToolCall   *ToolCall         `json:"tool_call,omitempty"`
	ToolResult *ToolResult       `json:"tool_result,omitempty"`
	Usage      *Usage            `json:"usage,omitempty"`
}

type ToolChatEventHandler func(ToolChatEvent) error

type ToolExecutor interface {
	GetAllTools() []ToolDefinition
	Execute(ctx context.Context, name string, arguments json.RawMessage) (any, error)
}

type ToolResult struct {
	Tool      string             `json:"tool"`
	Arguments any                `json:"arguments,omitempty"`
	Result    any                `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	Display   *ToolResultDisplay `json:"display,omitempty"`
}

func RunToolChat(
	ctx context.Context,
	provider Provider,
	model string,
	messages []Message,
	tools ToolExecutor,
	options ToolChatOptions,
) (*ChatResponse, []ToolCall, []ToolResult, []Message, error) {
	maxRounds := options.MaxRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolChatRounds
	}

	allToolCalls := make([]ToolCall, 0)
	allToolResults := make([]ToolResult, 0)
	transcript := make([]Message, 0)
	totalUsage := Usage{}

	for round := 0; round < maxRounds; round++ {
		if round > 0 {
			compactedMessages, compacted, compactionUsage, err := compactToolChatMessages(ctx, provider, model, messages, options.ContextWindow)
			if err != nil {
				return nil, allToolCalls, allToolResults, transcript, fmt.Errorf("compact tool chat context: %w", err)
			}
			if compacted {
				messages = compactedMessages
			}
			totalUsage.PromptTokens += compactionUsage.PromptTokens
			totalUsage.CompletionTokens += compactionUsage.CompletionTokens
			totalUsage.TotalTokens += compactionUsage.TotalTokens
			if exceeded, usedTokens := toolChatContextBudgetExceeded(messages, options.ContextWindow); exceeded {
				return nil, allToolCalls, allToolResults, transcript, fmt.Errorf(
					"tool execution reached the context window budget after %d rounds even after compaction (%s used)",
					round,
					FormatContextUsage(usedTokens, options.ContextWindow),
				)
			}
		}

		resp, err := provider.Chat(ctx, ChatRequest{
			Model:           model,
			Messages:        messages,
			Tools:           tools.GetAllTools(),
			ReasoningEffort: options.ReasoningEffort,
		})
		if err != nil {
			return nil, nil, nil, nil, err
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		toolCalls := normalizeToolCalls(resp.ToolCalls)
		if len(toolCalls) == 0 {
			resp.Usage = totalUsage
			finalMessage := Message{
				Role:      "assistant",
				Content:   resp.Content,
				Reasoning: resp.Reasoning,
			}
			transcript = append(transcript, finalMessage)
			return resp, allToolCalls, allToolResults, transcript, nil
		}

		allToolCalls = append(allToolCalls, toolCalls...)
		assistantMessage := Message{
			Role:      "assistant",
			Content:   resp.Content,
			Reasoning: resp.Reasoning,
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMessage)
		transcript = append(transcript, assistantMessage)

		for _, tc := range toolCalls {
			arguments := decodeJSONValue(tc.Function.Arguments)
			toolResult := ToolResult{
				Tool:      tc.Function.Name,
				Arguments: arguments,
			}

			result, err := tools.Execute(ctx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			payload := map[string]any{}
			if err != nil {
				toolResult.Error = err.Error()
				payload["error"] = err.Error()
			} else {
				normalizedResult, display := normalizeToolExecutionResult(result)
				toolResult.Result = normalizedResult
				toolResult.Display = display
				payload["result"] = normalizedResult
				if display != nil {
					payload["display"] = display
				}
			}

			allToolResults = append(allToolResults, toolResult)
			toolMessage := Message{
				Role:       "tool",
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    marshalToolPayload(payload),
			}
			messages = append(messages, toolMessage)
			transcript = append(transcript, toolMessage)
		}
	}

	return nil, allToolCalls, allToolResults, transcript, fmt.Errorf(
		"tool execution exceeded the safety limit of %d rounds",
		maxRounds,
	)
}

func RunToolChatStream(
	ctx context.Context,
	provider Provider,
	model string,
	messages []Message,
	tools ToolExecutor,
	options ToolChatOptions,
	handler ToolChatEventHandler,
) (*ChatResponse, []ToolCall, []ToolResult, []Message, error) {
	maxRounds := options.MaxRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolChatRounds
	}

	allToolCalls := make([]ToolCall, 0)
	allToolResults := make([]ToolResult, 0)
	transcript := make([]Message, 0)
	totalUsage := Usage{}

	for round := 0; round < maxRounds; round++ {
		if round > 0 {
			compactedMessages, compacted, compactionUsage, err := compactToolChatMessages(ctx, provider, model, messages, options.ContextWindow)
			if err != nil {
				return nil, allToolCalls, allToolResults, transcript, fmt.Errorf("compact tool chat context: %w", err)
			}
			if compacted {
				messages = compactedMessages
			}
			totalUsage.PromptTokens += compactionUsage.PromptTokens
			totalUsage.CompletionTokens += compactionUsage.CompletionTokens
			totalUsage.TotalTokens += compactionUsage.TotalTokens
			if exceeded, usedTokens := toolChatContextBudgetExceeded(messages, options.ContextWindow); exceeded {
				return nil, allToolCalls, allToolResults, transcript, fmt.Errorf(
					"tool execution reached the context window budget after %d rounds even after compaction (%s used)",
					round,
					FormatContextUsage(usedTokens, options.ContextWindow),
				)
			}
		}

		resp, err := ChatWithStream(ctx, provider, ChatRequest{
			Model:           model,
			Messages:        messages,
			Tools:           tools.GetAllTools(),
			ReasoningEffort: options.ReasoningEffort,
		}, func(event StreamEvent) error {
			if handler == nil {
				return nil
			}

			switch event.Type {
			case StreamEventContentDelta:
				return handler(ToolChatEvent{
					Type:  ToolChatEventContentDelta,
					Delta: event.Delta,
				})
			case StreamEventReasoningDelta:
				return handler(ToolChatEvent{
					Type:  ToolChatEventReasoningDelta,
					Delta: event.Delta,
				})
			case StreamEventUsage:
				usage := event.Usage
				return handler(ToolChatEvent{
					Type:  ToolChatEventUsage,
					Usage: &usage,
				})
			default:
				return nil
			}
		})
		if err != nil {
			return nil, nil, nil, nil, err
		}

		totalUsage.PromptTokens += resp.Usage.PromptTokens
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += resp.Usage.TotalTokens

		toolCalls := normalizeToolCalls(resp.ToolCalls)
		if len(toolCalls) == 0 {
			resp.Usage = totalUsage
			finalMessage := Message{
				Role:      "assistant",
				Content:   resp.Content,
				Reasoning: resp.Reasoning,
			}
			transcript = append(transcript, finalMessage)
			if handler != nil {
				usage := resp.Usage
				if err := handler(ToolChatEvent{
					Type:  ToolChatEventUsage,
					Usage: &usage,
				}); err != nil {
					return nil, nil, nil, nil, err
				}
			}
			return resp, allToolCalls, allToolResults, transcript, nil
		}

		allToolCalls = append(allToolCalls, toolCalls...)
		assistantMessage := Message{
			Role:      "assistant",
			Content:   resp.Content,
			Reasoning: resp.Reasoning,
			ToolCalls: toolCalls,
		}
		messages = append(messages, assistantMessage)
		transcript = append(transcript, assistantMessage)

		for _, tc := range toolCalls {
			arguments := decodeJSONValue(tc.Function.Arguments)
			toolResult := ToolResult{
				Tool:      tc.Function.Name,
				Arguments: arguments,
			}

			if handler != nil {
				toolCall := tc
				if err := handler(ToolChatEvent{
					Type:     ToolChatEventToolStarted,
					ToolCall: &toolCall,
				}); err != nil {
					return nil, nil, nil, nil, err
				}
			}

			result, err := tools.Execute(ctx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			payload := map[string]any{}
			if err != nil {
				toolResult.Error = err.Error()
				payload["error"] = err.Error()
			} else {
				normalizedResult, display := normalizeToolExecutionResult(result)
				toolResult.Result = normalizedResult
				toolResult.Display = display
				payload["result"] = normalizedResult
				if display != nil {
					payload["display"] = display
				}
			}

			allToolResults = append(allToolResults, toolResult)
			toolMessage := Message{
				Role:       "tool",
				Name:       tc.Function.Name,
				ToolCallID: tc.ID,
				Content:    marshalToolPayload(payload),
			}
			messages = append(messages, toolMessage)
			transcript = append(transcript, toolMessage)

			if handler != nil {
				toolCall := tc
				resultCopy := toolResult
				if err := handler(ToolChatEvent{
					Type:       ToolChatEventToolFinished,
					ToolCall:   &toolCall,
					ToolResult: &resultCopy,
				}); err != nil {
					return nil, nil, nil, nil, err
				}
			}
		}
	}

	return nil, allToolCalls, allToolResults, transcript, fmt.Errorf(
		"tool execution exceeded the safety limit of %d rounds",
		maxRounds,
	)
}

func normalizeToolCalls(toolCalls []ToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	normalized := make([]ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("tool-call-%d", i+1)
		}
		if tc.Type == "" {
			tc.Type = "function"
		}
		normalized[i] = tc
	}

	return normalized
}

func decodeJSONValue(raw string) any {
	if raw == "" {
		return nil
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}

	return raw
}

func marshalToolPayload(payload map[string]any) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		if errMessage, ok := payload["error"].(string); ok && errMessage != "" {
			return errMessage
		}
		return "{}"
	}

	return string(encoded)
}

func toolChatContextBudgetExceeded(messages []Message, contextWindow int) (bool, int) {
	if contextWindow <= 0 {
		return false, 0
	}

	usedTokens := EstimateMessagesTokens(messages)
	if usedTokens <= 0 {
		return false, 0
	}

	remainingTokens := contextWindow - usedTokens
	reserveTokens := maxInt(toolChatContextReserveMin, contextWindow/toolChatContextReserveFrac)
	if halfWindow := contextWindow / 2; halfWindow > 0 && reserveTokens > halfWindow {
		reserveTokens = halfWindow
	}

	thresholdTokens := int(float64(contextWindow) * toolChatContextThreshold)
	if usedTokens >= thresholdTokens || remainingTokens <= reserveTokens {
		return true, usedTokens
	}

	return false, usedTokens
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func compactToolChatMessages(
	ctx context.Context,
	provider Provider,
	model string,
	messages []Message,
	contextWindow int,
) ([]Message, bool, Usage, error) {
	if contextWindow <= 0 {
		return messages, false, Usage{}, nil
	}

	working := append([]Message(nil), messages...)
	compactedAny := false
	totalUsage := Usage{}

	for pass := 0; pass < toolChatCompactionPasses; pass++ {
		if exceeded, _ := toolChatContextBudgetExceeded(working, contextWindow); !exceeded {
			return working, compactedAny, totalUsage, nil
		}

		compacted, nextMessages, usage, err := compactToolChatMessagesPass(ctx, provider, model, working)
		if err != nil {
			return messages, false, totalUsage, err
		}
		if !compacted {
			return working, compactedAny, totalUsage, nil
		}

		compactedAny = true
		working = nextMessages
		totalUsage.PromptTokens += usage.PromptTokens
		totalUsage.CompletionTokens += usage.CompletionTokens
		totalUsage.TotalTokens += usage.TotalTokens
	}

	return working, compactedAny, totalUsage, nil
}

func compactToolChatMessagesPass(
	ctx context.Context,
	provider Provider,
	model string,
	messages []Message,
) (bool, []Message, Usage, error) {
	leadingSystem, existingSummary, body := splitToolChatLeadingMessages(messages)
	if len(body) <= 1 {
		return false, messages, Usage{}, nil
	}

	rawBuffer := toolChatRawMessageBuffer
	if len(body) <= rawBuffer {
		rawBuffer = maxInt(2, len(body)/2)
		if rawBuffer >= len(body) {
			rawBuffer = len(body) - 1
		}
	}
	if rawBuffer <= 0 {
		return false, messages, Usage{}, nil
	}

	compactUntil := len(body) - rawBuffer
	for compactUntil > 0 && compactUntil < len(body) && body[compactUntil].Role == "tool" {
		compactUntil--
	}
	if compactUntil <= 0 {
		return false, messages, Usage{}, nil
	}

	segment := body[:compactUntil]
	recent := body[compactUntil:]
	summary, usage, err := summarizeToolChatSegment(ctx, provider, model, existingSummary, segment)
	if err != nil {
		return false, nil, Usage{}, err
	}
	if strings.TrimSpace(summary) == "" {
		return false, messages, Usage{}, nil
	}

	nextMessages := append([]Message(nil), leadingSystem...)
	nextMessages = append(nextMessages, Message{
		Role:    "system",
		Content: toolChatSummaryPrefix + summary,
	})
	nextMessages = append(nextMessages, recent...)

	return true, nextMessages, usage, nil
}

func splitToolChatLeadingMessages(messages []Message) ([]Message, string, []Message) {
	index := 0
	leadingSystem := make([]Message, 0, len(messages))
	summaries := make([]string, 0, 1)

	for index < len(messages) && messages[index].Role == "system" {
		content := strings.TrimSpace(messages[index].Content)
		if strings.HasPrefix(content, toolChatSummaryPrefix) {
			summary := strings.TrimSpace(strings.TrimPrefix(content, toolChatSummaryPrefix))
			if summary != "" {
				summaries = append(summaries, summary)
			}
		} else {
			leadingSystem = append(leadingSystem, messages[index])
		}
		index++
	}

	return leadingSystem, strings.Join(summaries, "\n"), messages[index:]
}

func summarizeToolChatSegment(
	ctx context.Context,
	provider Provider,
	model string,
	existingSummary string,
	messages []Message,
) (string, Usage, error) {
	transcript := formatToolChatTranscript(messages)
	if strings.TrimSpace(transcript) == "" {
		return strings.TrimSpace(existingSummary), Usage{}, nil
	}

	userPrompt := strings.TrimSpace(strings.Join([]string{
		"Update the hidden running memory for this tool-assisted response.",
		formatToolChatSummary(existingSummary),
		"New transcript to compact:",
		transcript,
		"Return only the updated hidden memory as concise bullet points. Preserve the user goal, constraints, tool findings, partial progress, and unresolved work so the assistant can continue the same response.",
	}, "\n\n"))

	resp, err := provider.Chat(ctx, ChatRequest{
		Model: model,
		Messages: []Message{
			{
				Role:    "system",
				Content: "You write hidden running memory for a tool-using assistant while it is still answering the same user request. Keep it concise, factual, and reusable. Do not address the user directly.",
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		MaxTokens: toolChatSummaryMaxTokens,
	})
	if err != nil {
		return "", Usage{}, err
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", Usage{}, fmt.Errorf("empty summary")
	}

	return summary, resp.Usage, nil
}

func formatToolChatSummary(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return "Existing hidden memory:\n(none)"
	}
	return "Existing hidden memory:\n" + strings.TrimSpace(summary)
}

func formatToolChatTranscript(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, message := range messages {
		role := strings.ToUpper(strings.TrimSpace(message.Role))
		if role == "" {
			role = "MESSAGE"
		}

		builder.WriteString(role)
		builder.WriteString(": ")
		builder.WriteString(compactToolChatText(message.Content))
		builder.WriteByte('\n')

		if len(message.ToolCalls) > 0 {
			encoded, err := json.Marshal(message.ToolCalls)
			if err == nil {
				builder.WriteString("TOOL CALLS: ")
				builder.WriteString(compactToolChatText(string(encoded)))
				builder.WriteByte('\n')
			}
		}
	}

	return strings.TrimSpace(builder.String())
}

func compactToolChatText(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return "(empty)"
	}

	runes := []rune(normalized)
	if len(runes) <= toolChatSummaryMessageLimit {
		return normalized
	}

	return string(runes[:toolChatSummaryMessageLimit-1]) + "…"
}
