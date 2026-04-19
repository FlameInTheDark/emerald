package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/llm"
)

type llmChatStreamEvent struct {
	Type       string                         `json:"type"`
	Delta      string                         `json:"delta,omitempty"`
	Turn       *llmChatStreamTurnStartedEvent `json:"turn,omitempty"`
	ToolCall   *llm.ToolCall                  `json:"tool_call,omitempty"`
	ToolResult *llm.ToolResult                `json:"tool_result,omitempty"`
	Usage      *llm.Usage                     `json:"usage,omitempty"`
	Response   any                            `json:"response,omitempty"`
	Error      string                         `json:"error,omitempty"`
	Status     int                            `json:"status,omitempty"`
}

type llmChatStreamTurnStartedEvent struct {
	ConversationID   string                      `json:"conversation_id"`
	Conversation     conversationResponse        `json:"conversation"`
	UserMessage      conversationMessageResponse `json:"user_message"`
	AssistantMessage conversationMessageResponse `json:"assistant_message"`
}

type llmChatResponsePayload struct {
	ConversationID  string               `json:"conversation_id"`
	Conversation    conversationResponse `json:"conversation"`
	Content         string               `json:"content"`
	Reasoning       string               `json:"reasoning,omitempty"`
	ToolCalls       []llm.ToolCall       `json:"tool_calls,omitempty"`
	ToolResults     []llm.ToolResult     `json:"tool_results,omitempty"`
	ContextMessages []llm.Message        `json:"context_messages,omitempty"`
	Usage           llm.Usage            `json:"usage"`
}

func (h *LLMChatHandler) ChatStream(c *fiber.Ctx) error {
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
		status := llmErrorStatus(err)
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	streamCtx := context.Background()
	if userCtx := c.UserContext(); userCtx != nil {
		streamCtx = userCtx
	}
	persistCtx := context.WithoutCancel(streamCtx)

	turn, err := h.beginStreamingChatTurn(persistCtx, prepared, req.Message)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	turnMessages, err := decodeConversationMessages([]models.ChatMessage{*turn.userMessage, *turn.assistantMessage})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(writer *bufio.Writer) {
		startedEvent := llmChatStreamTurnStartedEvent{
			ConversationID:   turn.conversation.ID,
			Conversation:     newConversationResponse(*turn.conversation, nil),
			UserMessage:      turnMessages[0],
			AssistantMessage: turnMessages[1],
		}
		if err := writeLLMChatStreamEvent(writer, llmChatStreamEvent{
			Type: "turn_started",
			Turn: &startedEvent,
		}); err != nil {
			return
		}

		var partialUsage llm.Usage
		partialToolCalls := make([]llm.ToolCall, 0)
		partialToolResults := make([]llm.ToolResult, 0)
		partialContextMessages := make([]llm.Message, 0)

		persistPartialSnapshot := func() error {
			return h.persistStreamingChatTurn(persistCtx, prepared, turn, chatTurnSnapshot{
				response:        &llm.ChatResponse{Usage: partialUsage},
				toolCalls:       cloneToolCalls(partialToolCalls),
				toolResults:     cloneToolResults(partialToolResults),
				contextMessages: cloneContextMessages(partialContextMessages),
			})
		}

		resp, toolCalls, toolResults, contextMessages, streamErr := runToolChatStream(
			streamCtx,
			prepared.provider,
			prepared.providerConfig.Model,
			modelMessages,
			prepared.toolRegistry,
			derefString(prepared.conversation.ReasoningEffort),
			prepared.contextWindow,
			func(event llm.ToolChatEvent) error {
				switch event.Type {
				case llm.ToolChatEventContentDelta:
					partialContextMessages = appendAssistantContentDelta(partialContextMessages, event.Delta)
					if err := persistPartialSnapshot(); err != nil {
						return err
					}
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:  "assistant_delta",
						Delta: event.Delta,
					})
				case llm.ToolChatEventReasoningDelta:
					partialContextMessages = appendAssistantReasoningDelta(partialContextMessages, event.Delta)
					if err := persistPartialSnapshot(); err != nil {
						return err
					}
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:  "assistant_reasoning_delta",
						Delta: event.Delta,
					})
				case llm.ToolChatEventToolStarted:
					if event.ToolCall != nil {
						partialToolCalls = append(partialToolCalls, *event.ToolCall)
						partialContextMessages = appendAssistantToolCall(partialContextMessages, *event.ToolCall)
					}
					if err := persistPartialSnapshot(); err != nil {
						return err
					}
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:     "tool_started",
						ToolCall: event.ToolCall,
					})
				case llm.ToolChatEventToolFinished:
					if event.ToolResult != nil {
						partialToolResults = append(partialToolResults, *event.ToolResult)
					}
					if event.ToolResult != nil {
						partialContextMessages = appendToolResultMessage(partialContextMessages, event.ToolCall, *event.ToolResult)
					}
					if err := persistPartialSnapshot(); err != nil {
						return err
					}
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:       "tool_finished",
						ToolCall:   event.ToolCall,
						ToolResult: event.ToolResult,
					})
				case llm.ToolChatEventUsage:
					if event.Usage != nil {
						partialUsage = *event.Usage
					}
					if err := persistPartialSnapshot(); err != nil {
						return err
					}
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:  "usage",
						Usage: event.Usage,
					})
				default:
					return nil
				}
			},
		)

		if streamErr != nil {
			if len(contextMessages) > 0 {
				partialContextMessages = cloneContextMessages(contextMessages)
			}
			if len(toolCalls) > 0 {
				partialToolCalls = cloneToolCalls(toolCalls)
			}
			if len(toolResults) > 0 {
				partialToolResults = cloneToolResults(toolResults)
			}
			if resp != nil {
				partialUsage = resp.Usage
			}

			if err := persistPartialSnapshot(); err != nil {
				_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
					Type:   "error",
					Error:  err.Error(),
					Status: fiber.StatusInternalServerError,
				})
				return
			}

			_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
				Type:   "error",
				Error:  llmErrorMessage("LLM request failed", streamErr),
				Status: llmErrorStatus(streamErr),
			})
			return
		}

		finalSnapshot := chatTurnSnapshot{
			response:        resp,
			toolCalls:       toolCalls,
			toolResults:     toolResults,
			contextMessages: contextMessages,
		}
		if err := h.persistStreamingChatTurn(persistCtx, prepared, turn, finalSnapshot); err != nil {
			_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
				Type:   "error",
				Error:  err.Error(),
				Status: fiber.StatusInternalServerError,
			})
			return
		}

		_ = writeLLMChatStreamEvent(writer, llmChatStreamEvent{
			Type: "done",
			Response: llmChatResponsePayload{
				ConversationID:  turn.conversation.ID,
				Conversation:    newConversationResponse(*turn.conversation, nil),
				Content:         resp.Content,
				Reasoning:       resp.Reasoning,
				ToolCalls:       toolCalls,
				ToolResults:     toolResults,
				ContextMessages: contextMessages,
				Usage:           resp.Usage,
			},
		})
	})

	return nil
}

func writeLLMChatStreamEvent(writer *bufio.Writer, event llmChatStreamEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal stream event: %w", err)
	}

	if _, err := writer.WriteString("data: " + string(payload) + "\n\n"); err != nil {
		return fmt.Errorf("write stream event: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush stream event: %w", err)
	}
	return nil
}

func appendAssistantContentDelta(messages []llm.Message, delta string) []llm.Message {
	if strings.TrimSpace(delta) == "" {
		return messages
	}

	next := cloneContextMessages(messages)
	if last := len(next) - 1; last >= 0 && next[last].Role == "assistant" && len(next[last].ToolCalls) == 0 {
		next[last].Content += delta
		return next
	}

	return append(next, llm.Message{
		Role:    "assistant",
		Content: delta,
	})
}

func appendAssistantReasoningDelta(messages []llm.Message, delta string) []llm.Message {
	if strings.TrimSpace(delta) == "" {
		return messages
	}

	next := cloneContextMessages(messages)
	if last := len(next) - 1; last >= 0 && next[last].Role == "assistant" && len(next[last].ToolCalls) == 0 {
		next[last].Reasoning += delta
		return next
	}

	return append(next, llm.Message{
		Role:      "assistant",
		Reasoning: delta,
	})
}

func appendAssistantToolCall(messages []llm.Message, toolCall llm.ToolCall) []llm.Message {
	next := cloneContextMessages(messages)
	if last := len(next) - 1; last >= 0 && next[last].Role == "assistant" {
		next[last].ToolCalls = append(next[last].ToolCalls, toolCall)
		return next
	}

	return append(next, llm.Message{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{toolCall},
	})
}

func appendToolResultMessage(messages []llm.Message, toolCall *llm.ToolCall, toolResult llm.ToolResult) []llm.Message {
	next := cloneContextMessages(messages)
	toolCallID := ""
	toolName := toolResult.Tool
	if toolCall != nil {
		toolCallID = strings.TrimSpace(toolCall.ID)
		if toolName == "" {
			toolName = toolCall.Function.Name
		}
	}

	toolMessage := llm.Message{
		Role:       "tool",
		Name:       toolName,
		ToolCallID: toolCallID,
		Content:    marshalToolResultContent(toolResult),
	}

	if toolCallID != "" {
		for index := range next {
			if next[index].Role == "tool" && next[index].ToolCallID == toolCallID {
				next[index] = toolMessage
				return next
			}
		}
	}

	return append(next, toolMessage)
}

func marshalToolResultContent(toolResult llm.ToolResult) string {
	payload := map[string]any{}
	if strings.TrimSpace(toolResult.Error) != "" {
		payload["error"] = toolResult.Error
	} else {
		payload["result"] = toolResult.Result
	}
	if toolResult.Display != nil {
		payload["display"] = toolResult.Display
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		if errorMessage, ok := payload["error"].(string); ok && errorMessage != "" {
			return errorMessage
		}
		return "{}"
	}
	return string(encoded)
}

func cloneContextMessages(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]llm.Message, len(messages))
	for index, message := range messages {
		cloned[index] = message
		if len(message.ToolCalls) > 0 {
			cloned[index].ToolCalls = cloneToolCalls(message.ToolCalls)
		}
	}
	return cloned
}

func cloneToolCalls(toolCalls []llm.ToolCall) []llm.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	cloned := make([]llm.ToolCall, len(toolCalls))
	copy(cloned, toolCalls)
	return cloned
}

func cloneToolResults(toolResults []llm.ToolResult) []llm.ToolResult {
	if len(toolResults) == 0 {
		return nil
	}
	cloned := make([]llm.ToolResult, len(toolResults))
	copy(cloned, toolResults)
	return cloned
}
