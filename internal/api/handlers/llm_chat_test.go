package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/llm"
)

type mockChatProvider struct {
	responses []*llm.ChatResponse
	requests  []llm.ChatRequest
}

func (m *mockChatProvider) Chat(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.requests = append(m.requests, req)
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp, nil
}

func (m *mockChatProvider) Name() string {
	return "mock"
}

func (m *mockChatProvider) Type() llm.ProviderType {
	return llm.ProviderCustom
}

type mockToolExecutor struct {
	definitions []llm.ToolDefinition
	executeFn   func(ctx context.Context, name string, arguments json.RawMessage) (any, error)
}

func (m *mockToolExecutor) GetAllTools() []llm.ToolDefinition {
	return m.definitions
}

func (m *mockToolExecutor) Execute(ctx context.Context, name string, arguments json.RawMessage) (any, error) {
	return m.executeFn(ctx, name, arguments)
}

func TestRunToolChatContinuesAfterToolExecution(t *testing.T) {
	provider := &mockChatProvider{
		responses: []*llm.ChatResponse{
			{
				ToolCalls: []llm.ToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Function: llm.ToolFunction{
							Name:      "list_nodes",
							Arguments: `{"cluster_name":"Local"}`,
						},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
			},
			{
				Content: "Cluster Local has 1 node.",
				Usage:   llm.Usage{PromptTokens: 12, CompletionTokens: 6, TotalTokens: 18},
			},
		},
	}

	tools := &mockToolExecutor{
		definitions: []llm.ToolDefinition{{Type: "function"}},
		executeFn: func(_ context.Context, name string, arguments json.RawMessage) (any, error) {
			if name != "list_nodes" {
				t.Fatalf("unexpected tool name: %s", name)
			}

			var decoded map[string]string
			if err := json.Unmarshal(arguments, &decoded); err != nil {
				t.Fatalf("unmarshal arguments: %v", err)
			}
			if decoded["cluster_name"] != "Local" {
				t.Fatalf("unexpected cluster_name: %q", decoded["cluster_name"])
			}

			return map[string]any{"nodes": []map[string]any{{"name": "pve"}}}, nil
		},
	}

	resp, toolCalls, toolResults, transcript, err := runToolChat(context.Background(), provider, "test-model", []llm.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "List nodes"},
	}, tools)
	if err != nil {
		t.Fatalf("runToolChat returned error: %v", err)
	}

	if resp.Content != "Cluster Local has 1 node." {
		t.Fatalf("unexpected final content: %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 32 {
		t.Fatalf("expected accumulated usage total 32, got %d", resp.Usage.TotalTokens)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if len(toolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(toolResults))
	}
	if len(transcript) != 3 {
		t.Fatalf("expected replay transcript with 3 messages, got %d", len(transcript))
	}
	if toolResults[0].Tool != "list_nodes" {
		t.Fatalf("unexpected tool result name: %s", toolResults[0].Tool)
	}

	if len(provider.requests) != 2 {
		t.Fatalf("expected 2 chat requests, got %d", len(provider.requests))
	}

	secondMessages := provider.requests[1].Messages
	if len(secondMessages) != 4 {
		t.Fatalf("expected 4 messages in follow-up request, got %d", len(secondMessages))
	}
	if secondMessages[2].Role != "assistant" || len(secondMessages[2].ToolCalls) != 1 {
		t.Fatalf("assistant tool-call message missing from follow-up request")
	}
	if secondMessages[3].Role != "tool" || secondMessages[3].ToolCallID != "call-1" {
		t.Fatalf("tool result message missing from follow-up request")
	}
}

func TestRunToolChatReturnsToolErrorsForFollowUp(t *testing.T) {
	provider := &mockChatProvider{
		responses: []*llm.ChatResponse{
			{
				ToolCalls: []llm.ToolCall{
					{
						Function: llm.ToolFunction{
							Name:      "list_nodes",
							Arguments: `{"cluster_name":"Local"}`,
						},
					},
				},
			},
			{
				Content: "The tool failed because authentication was rejected.",
			},
		},
	}

	tools := &mockToolExecutor{
		executeFn: func(_ context.Context, _ string, _ json.RawMessage) (any, error) {
			return nil, context.DeadlineExceeded
		},
	}

	resp, _, toolResults, transcript, err := runToolChat(context.Background(), provider, "test-model", []llm.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "List nodes"},
	}, tools)
	if err != nil {
		t.Fatalf("runToolChat returned error: %v", err)
	}

	if resp.Content == "" {
		t.Fatal("expected final assistant content after tool error")
	}
	if len(toolResults) != 1 || toolResults[0].Error == "" {
		t.Fatalf("expected surfaced tool error, got %+v", toolResults)
	}
	if len(transcript) != 3 {
		t.Fatalf("expected replay transcript with 3 messages, got %d", len(transcript))
	}
	if provider.requests[1].Messages[3].ToolCallID == "" {
		t.Fatal("expected synthesized tool call id for tool message")
	}
}
