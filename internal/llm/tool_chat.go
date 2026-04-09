package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

const DefaultMaxToolChatRounds = 8

type ToolChatEventType string

const (
	ToolChatEventContentDelta ToolChatEventType = "content_delta"
	ToolChatEventToolStarted  ToolChatEventType = "tool_started"
	ToolChatEventToolFinished ToolChatEventType = "tool_finished"
	ToolChatEventUsage        ToolChatEventType = "usage"
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
	Tool      string `json:"tool"`
	Arguments any    `json:"arguments,omitempty"`
	Result    any    `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

func RunToolChat(
	ctx context.Context,
	provider Provider,
	model string,
	messages []Message,
	tools ToolExecutor,
	maxRounds int,
) (*ChatResponse, []ToolCall, []ToolResult, []Message, error) {
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolChatRounds
	}

	allToolCalls := make([]ToolCall, 0)
	allToolResults := make([]ToolResult, 0)
	transcript := make([]Message, 0)
	totalUsage := Usage{}

	for round := 0; round < maxRounds; round++ {
		resp, err := provider.Chat(ctx, ChatRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools.GetAllTools(),
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
				Role:    "assistant",
				Content: resp.Content,
			}
			transcript = append(transcript, finalMessage)
			return resp, allToolCalls, allToolResults, transcript, nil
		}

		allToolCalls = append(allToolCalls, toolCalls...)
		assistantMessage := Message{
			Role:      "assistant",
			Content:   resp.Content,
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
				toolResult.Result = result
				payload["result"] = result
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

	return nil, allToolCalls, allToolResults, transcript, fmt.Errorf("tool execution exceeded %d rounds", maxRounds)
}

func RunToolChatStream(
	ctx context.Context,
	provider Provider,
	model string,
	messages []Message,
	tools ToolExecutor,
	maxRounds int,
	handler ToolChatEventHandler,
) (*ChatResponse, []ToolCall, []ToolResult, []Message, error) {
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolChatRounds
	}

	allToolCalls := make([]ToolCall, 0)
	allToolResults := make([]ToolResult, 0)
	transcript := make([]Message, 0)
	totalUsage := Usage{}

	for round := 0; round < maxRounds; round++ {
		resp, err := ChatWithStream(ctx, provider, ChatRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools.GetAllTools(),
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
				Role:    "assistant",
				Content: resp.Content,
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
				toolResult.Result = result
				payload["result"] = result
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

	return nil, allToolCalls, allToolResults, transcript, fmt.Errorf("tool execution exceeded %d rounds", maxRounds)
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
