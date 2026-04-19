package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type toolChatMockProvider struct {
	responses []*ChatResponse
	requests  []ChatRequest
	chatFn    func(req ChatRequest) (*ChatResponse, error)
}

func (m *toolChatMockProvider) Chat(_ context.Context, req ChatRequest) (*ChatResponse, error) {
	m.requests = append(m.requests, req)
	if m.chatFn != nil {
		return m.chatFn(req)
	}
	if len(m.responses) == 0 {
		return nil, context.Canceled
	}

	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}

func (m *toolChatMockProvider) Name() string {
	return "mock"
}

func (m *toolChatMockProvider) Type() ProviderType {
	return ProviderCustom
}

type toolChatMockExecutor struct {
	executeFn func(ctx context.Context, name string, arguments json.RawMessage) (any, error)
}

func (m *toolChatMockExecutor) GetAllTools() []ToolDefinition {
	return []ToolDefinition{{
		Type: "function",
		Function: ToolSpec{
			Name:        "search_web",
			Description: "Search the web.",
			Parameters:  map[string]any{"type": "object"},
		},
	}}
}

func (m *toolChatMockExecutor) Execute(ctx context.Context, name string, arguments json.RawMessage) (any, error) {
	return m.executeFn(ctx, name, arguments)
}

func TestRunToolChatAllowsMoreThanEightRounds(t *testing.T) {
	t.Parallel()

	responses := make([]*ChatResponse, 0, 10)
	for round := 0; round < 9; round++ {
		responses = append(responses, &ChatResponse{
			ToolCalls: []ToolCall{{
				ID:   "call-" + string(rune('1'+round)),
				Type: "function",
				Function: ToolFunction{
					Name:      "search_web",
					Arguments: `{"query":"round"}`,
				},
			}},
			Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}
	responses = append(responses, &ChatResponse{
		Content: "Finished after a long tool run.",
		Usage:   Usage{PromptTokens: 12, CompletionTokens: 6, TotalTokens: 18},
	})

	provider := &toolChatMockProvider{responses: responses}
	tools := &toolChatMockExecutor{
		executeFn: func(_ context.Context, name string, arguments json.RawMessage) (any, error) {
			if name != "search_web" {
				t.Fatalf("unexpected tool name: %s", name)
			}
			if strings.TrimSpace(string(arguments)) == "" {
				t.Fatal("expected tool arguments")
			}
			return map[string]any{"ok": true}, nil
		},
	}

	resp, toolCalls, toolResults, transcript, err := RunToolChat(
		context.Background(),
		provider,
		"test-model",
		[]Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Keep going until you are done."},
		},
		tools,
		ToolChatOptions{ContextWindow: DefaultContextWindow},
	)
	if err != nil {
		t.Fatalf("RunToolChat returned error: %v", err)
	}

	if resp.Content != "Finished after a long tool run." {
		t.Fatalf("unexpected final content: %q", resp.Content)
	}
	if len(provider.requests) != 10 {
		t.Fatalf("provider request count = %d, want 10", len(provider.requests))
	}
	if len(toolCalls) != 9 {
		t.Fatalf("tool call count = %d, want 9", len(toolCalls))
	}
	if len(toolResults) != 9 {
		t.Fatalf("tool result count = %d, want 9", len(toolResults))
	}
	if len(transcript) != 19 {
		t.Fatalf("transcript count = %d, want 19", len(transcript))
	}
}

func TestRunToolChatAutoCompactsWhenApproachingContextLimit(t *testing.T) {
	t.Parallel()

	toolRounds := 0
	provider := &toolChatMockProvider{
		chatFn: func(req ChatRequest) (*ChatResponse, error) {
			hasHiddenSummary := false
			for _, message := range req.Messages {
				if message.Role == "system" && strings.Contains(message.Content, "Hidden tool-run memory from earlier in this response:") {
					hasHiddenSummary = true
					break
				}
			}

			switch {
			case len(req.Tools) == 0:
				return &ChatResponse{
					Content: "- User wants the search to continue\n- Earlier searches already covered the first set of results",
					Usage:   Usage{PromptTokens: 8, CompletionTokens: 4, TotalTokens: 12},
				}, nil
			case hasHiddenSummary:
				return &ChatResponse{
					Content: "Done after compacting older tool context.",
					Usage:   Usage{PromptTokens: 12, CompletionTokens: 6, TotalTokens: 18},
				}, nil
			default:
				toolRounds++
				return &ChatResponse{
					ToolCalls: []ToolCall{{
						ID:   "call-" + string(rune('0'+toolRounds)),
						Type: "function",
						Function: ToolFunction{
							Name:      "search_web",
							Arguments: `{"query":"` + strings.Repeat("x", 160) + `"}`,
						},
					}},
					Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
				}, nil
			}
		},
	}
	tools := &toolChatMockExecutor{
		executeFn: func(_ context.Context, _ string, _ json.RawMessage) (any, error) {
			return map[string]any{"content": strings.Repeat("y", 220)}, nil
		},
	}

	resp, toolCalls, toolResults, transcript, err := RunToolChat(
		context.Background(),
		provider,
		"test-model",
		[]Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Use tools carefully."},
		},
		tools,
		ToolChatOptions{ContextWindow: 900},
	)
	if err != nil {
		t.Fatalf("RunToolChat returned error: %v", err)
	}
	if resp.Content != "Done after compacting older tool context." {
		t.Fatalf("unexpected final content: %q", resp.Content)
	}
	if len(provider.requests) < 4 {
		t.Fatalf("provider request count = %d, want at least 4", len(provider.requests))
	}
	if len(toolCalls) == 0 {
		t.Fatal("expected at least one tool call before compaction")
	}
	if len(toolResults) != len(toolCalls) {
		t.Fatalf("tool result count = %d, want %d", len(toolResults), len(toolCalls))
	}
	if len(transcript) != len(toolCalls)*2+1 {
		t.Fatalf("transcript count = %d, want %d", len(transcript), len(toolCalls)*2+1)
	}

	summaryRequest := provider.requests[len(provider.requests)-2]
	if len(summaryRequest.Tools) != 0 {
		t.Fatalf("expected compaction request without tools, got %+v", summaryRequest.Tools)
	}
	if len(summaryRequest.Messages) != 2 || summaryRequest.Messages[0].Role != "system" {
		t.Fatalf("unexpected compaction request messages: %+v", summaryRequest.Messages)
	}

	finalRequest := provider.requests[len(provider.requests)-1]
	foundSummary := false
	for _, message := range finalRequest.Messages {
		if message.Role == "system" && strings.Contains(message.Content, "Hidden tool-run memory from earlier in this response:") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("expected compacted summary in final request, got %+v", finalRequest.Messages)
	}
}
