package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveContextWindowWithDiscoveryUsesLMStudioLoadedContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if _, err := fmt.Fprint(w, `{"models":[{"key":"openai/gpt-oss-20b","display_name":"GPT OSS 20B","loaded_instances":[{"id":"local-gpt-oss","config":{"context_length":16384}}],"max_context_length":131072}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	contextWindow := ResolveContextWindowWithDiscovery(context.Background(), Config{
		ProviderType: ProviderLMStudio,
		BaseURL:      server.URL + "/v1",
		Model:        "local-gpt-oss",
	})

	if contextWindow != 16384 {
		t.Fatalf("context window = %d, want 16384", contextWindow)
	}
}

func TestResolveContextWindowWithDiscoveryFallsBackWhenModelIsUnknown(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"models":[{"key":"openai/gpt-oss-20b","display_name":"GPT OSS 20B","loaded_instances":[],"max_context_length":131072}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	contextWindow := ResolveContextWindowWithDiscovery(context.Background(), Config{
		ProviderType: ProviderLMStudio,
		BaseURL:      server.URL + "/v1",
		Model:        "missing-model",
	})

	if contextWindow != DefaultOllamaContextWindow {
		t.Fatalf("context window = %d, want %d", contextWindow, DefaultOllamaContextWindow)
	}
}
