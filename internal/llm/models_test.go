package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListModelsOpenRouter(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if _, err := fmt.Fprint(w, `{"data":[{"id":"openai/gpt-4o-mini","name":"GPT-4o mini","context_length":128000},{"id":"anthropic/claude-3.7-sonnet","name":"Claude 3.7 Sonnet","context_length":200000}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	models, err := ListModels(context.Background(), Config{
		ProviderType: ProviderOpenRouter,
		APIKey:       "secret",
		BaseURL:      server.URL,
	})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].ID != "anthropic/claude-3.7-sonnet" || models[1].ID != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected model order: %#v", models)
	}
}

func TestListModelsOllama(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if _, err := fmt.Fprint(w, `{"models":[{"name":"llama3.2"},{"name":"qwen2.5"}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	models, err := ListModels(context.Background(), Config{
		ProviderType: ProviderOllama,
		BaseURL:      server.URL,
	})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].ID != "llama3.2" || models[1].ID != "qwen2.5" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestListModelsCustomProvider(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if _, err := fmt.Fprint(w, `{"data":[{"id":"custom/alpha","name":"Alpha"},{"id":"custom/beta","name":"Beta"}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	models, err := ListModels(context.Background(), Config{
		ProviderType: ProviderCustom,
		APIKey:       "secret",
		BaseURL:      server.URL,
	})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].ID != "custom/alpha" || models[1].ID != "custom/beta" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestListModelsCustomProviderWithChatEndpointBaseURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if _, err := fmt.Fprint(w, `{"data":[{"id":"custom/reasoner","name":"Reasoner"}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	models, err := ListModels(context.Background(), Config{
		ProviderType: ProviderCustom,
		BaseURL:      server.URL + "/v1/chat/completions",
	})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if len(models) != 1 || models[0].ID != "custom/reasoner" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestListModelsLMStudioUsesRESTModelsEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		if _, err := fmt.Fprint(w, `{"models":[{"key":"openai/gpt-oss-20b","display_name":"GPT OSS 20B","loaded_instances":[{"id":"local-gpt-oss","config":{"context_length":24576}}],"max_context_length":131072},{"key":"qwen/qwen3-4b","display_name":"Qwen 3 4B","loaded_instances":[],"max_context_length":32768}]}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	models, err := ListModels(context.Background(), Config{
		ProviderType: ProviderLMStudio,
		BaseURL:      server.URL + "/v1/chat/completions",
	})
	if err != nil {
		t.Fatalf("ListModels returned error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if models[0].ID != "local-gpt-oss" || models[0].ContextLength != 24576 {
		t.Fatalf("unexpected loaded instance: %#v", models[0])
	}
	if models[1].ID != "qwen/qwen3-4b" || models[1].ContextLength != 32768 {
		t.Fatalf("unexpected unloaded model: %#v", models[1])
	}
}
