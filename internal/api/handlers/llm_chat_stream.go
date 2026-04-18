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

	"github.com/FlameInTheDark/emerald/internal/llm"
)

type llmChatStreamEvent struct {
	Type       string          `json:"type"`
	Delta      string          `json:"delta,omitempty"`
	ToolCall   *llm.ToolCall   `json:"tool_call,omitempty"`
	ToolResult *llm.ToolResult `json:"tool_result,omitempty"`
	Usage      *llm.Usage      `json:"usage,omitempty"`
	Response   any             `json:"response,omitempty"`
	Error      string          `json:"error,omitempty"`
	Status     int             `json:"status,omitempty"`
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

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set(fiber.HeaderConnection, "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	c.Context().SetBodyStreamWriter(func(writer *bufio.Writer) {
		resp, toolCalls, toolResults, contextMessages, err := runToolChatStream(
			streamCtx,
			prepared.provider,
			prepared.providerConfig.Model,
			modelMessages,
			prepared.toolRegistry,
			derefString(prepared.conversation.ReasoningEffort),
			func(event llm.ToolChatEvent) error {
				switch event.Type {
				case llm.ToolChatEventContentDelta:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:  "assistant_delta",
						Delta: event.Delta,
					})
				case llm.ToolChatEventReasoningDelta:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:  "assistant_reasoning_delta",
						Delta: event.Delta,
					})
				case llm.ToolChatEventToolStarted:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:     "tool_started",
						ToolCall: event.ToolCall,
					})
				case llm.ToolChatEventToolFinished:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:       "tool_finished",
						ToolCall:   event.ToolCall,
						ToolResult: event.ToolResult,
					})
				case llm.ToolChatEventUsage:
					return writeLLMChatStreamEvent(writer, llmChatStreamEvent{
						Type:  "usage",
						Usage: event.Usage,
					})
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

		reloaded, err := h.persistChatTurn(streamCtx, prepared, req.Message, resp, toolCalls, toolResults, contextMessages)
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
			Response: llmChatResponsePayload{
				ConversationID:  prepared.conversation.ID,
				Conversation:    newConversationResponse(*reloaded, nil),
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
