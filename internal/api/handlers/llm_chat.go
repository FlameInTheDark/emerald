package handlers

import (
	"context"

	"github.com/FlameInTheDark/automator/internal/llm"
)

const maxToolChatRounds = llm.DefaultMaxToolChatRounds

func runToolChat(
	ctx context.Context,
	provider llm.Provider,
	model string,
	messages []llm.Message,
	tools llm.ToolExecutor,
) (*llm.ChatResponse, []llm.ToolCall, []llm.ToolResult, []llm.Message, error) {
	return llm.RunToolChat(ctx, provider, model, messages, tools, maxToolChatRounds)
}

func runToolChatStream(
	ctx context.Context,
	provider llm.Provider,
	model string,
	messages []llm.Message,
	tools llm.ToolExecutor,
	handler llm.ToolChatEventHandler,
) (*llm.ChatResponse, []llm.ToolCall, []llm.ToolResult, []llm.Message, error) {
	return llm.RunToolChatStream(ctx, provider, model, messages, tools, maxToolChatRounds, handler)
}
