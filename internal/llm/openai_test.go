package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveOpenAICompatibleChatEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{
			name:     "root API base",
			baseURL:  "https://example.com/v1",
			expected: "https://example.com/v1/chat/completions",
		},
		{
			name:     "full chat endpoint",
			baseURL:  "https://example.com/v1/chat/completions",
			expected: "https://example.com/v1/chat/completions",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveOpenAICompatibleChatEndpoint(test.baseURL, ProviderCustom); got != test.expected {
				t.Fatalf("chat endpoint = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestResolveOpenAICompatibleModelsEndpointFromChatEndpoint(t *testing.T) {
	t.Parallel()

	got := resolveOpenAICompatibleModelsEndpoint("https://example.com/v1/chat/completions", ProviderCustom)
	want := "https://example.com/v1/models"
	if got != want {
		t.Fatalf("models endpoint = %q, want %q", got, want)
	}
}

func TestCustomProviderChatNormalizesRootBaseURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if _, err := fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	provider, err := NewCustomProvider(Config{
		ProviderType: ProviderCustom,
		BaseURL:      server.URL + "/v1",
		Model:        "test-model",
	})
	if err != nil {
		t.Fatalf("NewCustomProvider returned error: %v", err)
	}

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "test-model",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("unexpected response content: %q", resp.Content)
	}
}

func TestOpenAICompatibleChatParsesReasoning(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","reasoning_content":"Think first","content":"Final answer"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	provider, err := NewCustomProvider(Config{
		ProviderType: ProviderCustom,
		BaseURL:      server.URL + "/v1/chat/completions",
		Model:        "reasoner",
	})
	if err != nil {
		t.Fatalf("NewCustomProvider returned error: %v", err)
	}

	resp, err := provider.Chat(context.Background(), ChatRequest{
		Model:    "reasoner",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Reasoning != "Think first" {
		t.Fatalf("reasoning = %q, want %q", resp.Reasoning, "Think first")
	}
}

func TestOpenAICompatibleChatStreamParsesReasoning(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected streaming response writer")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		for _, payload := range []string{
			`{"choices":[{"delta":{"reasoning_content":"Step 1. "},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"reasoning":"Step 2."},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":"Done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
			`[DONE]`,
		} {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				t.Fatalf("write response: %v", err)
			}
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider, err := NewCustomProvider(Config{
		ProviderType: ProviderCustom,
		BaseURL:      server.URL + "/v1",
		Model:        "reasoner",
	})
	if err != nil {
		t.Fatalf("NewCustomProvider returned error: %v", err)
	}

	events := make([]StreamEvent, 0, 3)
	resp, err := provider.ChatStream(context.Background(), ChatRequest{
		Model:    "reasoner",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream returned error: %v", err)
	}

	if resp.Reasoning != "Step 1. Step 2." {
		t.Fatalf("reasoning = %q, want %q", resp.Reasoning, "Step 1. Step 2.")
	}
	if resp.Content != "Done" {
		t.Fatalf("content = %q, want %q", resp.Content, "Done")
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 stream events, got %d", len(events))
	}
	if events[0].Type != StreamEventReasoningDelta || events[0].Delta != "Step 1. " {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Type != StreamEventReasoningDelta || events[1].Delta != "Step 2." {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if events[2].Type != StreamEventContentDelta || events[2].Delta != "Done" {
		t.Fatalf("unexpected third event: %+v", events[2])
	}
	if events[3].Type != StreamEventUsage || events[3].Usage.TotalTokens != 3 {
		t.Fatalf("unexpected fourth event: %+v", events[3])
	}
}
