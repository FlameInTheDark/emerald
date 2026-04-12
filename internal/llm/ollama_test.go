package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildOllamaRequestEncodesAssistantToolArgumentsAsObject(t *testing.T) {
	t.Parallel()

	apiReq := buildOllamaRequest(ChatRequest{
		Model: "llama3.2",
		Messages: []Message{
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Function: ToolFunction{
							Name:      "list_nodes",
							Arguments: `{"cluster_name":"Local"}`,
						},
					},
				},
			},
		},
	}, true)

	if len(apiReq.Messages) != 1 || len(apiReq.Messages[0].ToolCalls) != 1 {
		t.Fatalf("unexpected request messages: %#v", apiReq.Messages)
	}

	arguments, ok := apiReq.Messages[0].ToolCalls[0].Function.Arguments.(map[string]any)
	if !ok {
		t.Fatalf("expected ollama tool arguments to decode into an object, got %T", apiReq.Messages[0].ToolCalls[0].Function.Arguments)
	}
	if got := arguments["cluster_name"]; got != "Local" {
		t.Fatalf("unexpected cluster_name: %#v", got)
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if !strings.Contains(string(body), `"arguments":{"cluster_name":"Local"}`) {
		t.Fatalf("expected request body to contain object arguments, got %s", string(body))
	}
}

func TestBuildOllamaRequestPreservesToolErrorContent(t *testing.T) {
	t.Parallel()

	apiReq := buildOllamaRequest(ChatRequest{
		Model: "llama3.2",
		Messages: []Message{
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Function: ToolFunction{
							Name:      "list_nodes",
							Arguments: `{"cluster_name":"wrong"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Name:       "list_nodes",
				ToolCallID: "call-1",
				Content:    `{"error":"cluster \"wrong\" not found"}`,
			},
		},
	}, true)

	if len(apiReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(apiReq.Messages))
	}
	if apiReq.Messages[1].Role != "tool" {
		t.Fatalf("expected follow-up message role to be tool, got %q", apiReq.Messages[1].Role)
	}
	if apiReq.Messages[1].Content != `{"error":"cluster \"wrong\" not found"}` {
		t.Fatalf("unexpected tool error content: %q", apiReq.Messages[1].Content)
	}
	if apiReq.Messages[1].ToolCallID != "call-1" {
		t.Fatalf("unexpected tool call id: %q", apiReq.Messages[1].ToolCallID)
	}
}

func TestOllamaProviderChatParsesObjectToolArguments(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}

		fmt.Fprint(w, `{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"list_nodes","arguments":{"cluster_name":"Local"}}}]},"prompt_eval_count":11,"eval_count":5}`)
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(Config{
		ProviderType: ProviderOllama,
		BaseURL:      server.URL,
		Model:        "llama3.2",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider returned error: %v", err)
	}

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model: "llama3.2",
		Messages: []Message{
			{Role: "user", Content: "List nodes"},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "list_nodes" {
		t.Fatalf("unexpected tool call name: %s", resp.ToolCalls[0].Function.Name)
	}
	if resp.ToolCalls[0].Function.Arguments != `{"cluster_name":"Local"}` {
		t.Fatalf("unexpected tool call arguments: %s", resp.ToolCalls[0].Function.Arguments)
	}
}

func TestOllamaProviderChatStreamParsesObjectToolArguments(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected streaming response writer")
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"I'll start"},"done":false}`)
		flusher.Flush()
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":" by reading the relevant"},"done":false}`)
		flusher.Flush()
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"list_nodes","arguments":{"cluster_name":"Local"}}}]},"done":false}`)
		flusher.Flush()
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":11,"eval_count":5}`)
		flusher.Flush()
	}))
	defer server.Close()

	provider, err := NewOllamaProvider(Config{
		ProviderType: ProviderOllama,
		BaseURL:      server.URL,
		Model:        "llama3.2",
	})
	if err != nil {
		t.Fatalf("NewOllamaProvider returned error: %v", err)
	}

	events := make([]StreamEvent, 0, 3)
	resp, err := provider.ChatStream(context.Background(), ChatRequest{
		Model: "llama3.2",
		Messages: []Message{
			{Role: "user", Content: "List nodes"},
		},
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	if resp.Content != "I'll start by reading the relevant" {
		t.Fatalf("unexpected streamed content: %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Arguments != `{"cluster_name":"Local"}` {
		t.Fatalf("unexpected tool call arguments: %s", resp.ToolCalls[0].Function.Arguments)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 stream events, got %d", len(events))
	}
	if events[0].Type != StreamEventContentDelta || events[0].Delta != "I'll start" {
		t.Fatalf("unexpected first stream event: %+v", events[0])
	}
	if events[1].Type != StreamEventContentDelta || events[1].Delta != " by reading the relevant" {
		t.Fatalf("unexpected second stream event: %+v", events[1])
	}
	if events[2].Type != StreamEventUsage || events[2].Usage.TotalTokens != 16 {
		t.Fatalf("unexpected usage event: %+v", events[2])
	}
}
