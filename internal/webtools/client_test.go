package webtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSearchSearXNGParsesStructuredResults(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("path = %q, want /search", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "emerald automation" {
			t.Fatalf("query = %q, want emerald automation", got)
		}
		if got := r.URL.Query().Get("format"); got != "json" {
			t.Fatalf("format = %q, want json", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"title":   "Emerald Docs",
					"url":     "https://example.com/docs",
					"content": "Automation docs and guides",
					"engine":  "duckduckgo",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.Client())
	resp, err := client.Search(context.Background(), RuntimeConfig{
		Config: Config{
			SearchProvider: SearchProviderSearXNG,
			SearXNGBaseURL: server.URL,
		},
	}, SearchRequest{
		Query: "emerald automation",
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("client.Search: %v", err)
	}

	if resp.Provider != SearchProviderSearXNG {
		t.Fatalf("provider = %q, want %q", resp.Provider, SearchProviderSearXNG)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("result count = %d, want 1", len(resp.Results))
	}
	if resp.Results[0].Title != "Emerald Docs" {
		t.Fatalf("result title = %q, want Emerald Docs", resp.Results[0].Title)
	}
}

func TestClientSearchJinaSendsAuthorizationHeader(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer secret-token" {
			t.Fatalf("authorization = %q, want Bearer secret-token", auth)
		}
		if got := r.URL.Query().Get("q"); got != "latest emerald" {
			t.Fatalf("query = %q, want latest emerald", got)
		}
		_, _ = w.Write([]byte("Search Result One\nSearch Result Two"))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	resp, err := client.Search(context.Background(), RuntimeConfig{
		Config: Config{
			SearchProvider:    SearchProviderJina,
			JinaSearchBaseURL: server.URL,
		},
		JinaAPIKey: "secret-token",
	}, SearchRequest{
		Query: "latest emerald",
	})
	if err != nil {
		t.Fatalf("client.Search: %v", err)
	}

	if !strings.Contains(resp.Content, "Search Result One") {
		t.Fatalf("unexpected Jina search content %q", resp.Content)
	}
}

func TestClientOpenPageWithHTTPExtractsReadableText(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Docs</title></head><body><main><h1>Emerald</h1><p>Ship automation fast.</p></main></body></html>`))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	resp, err := client.OpenPage(context.Background(), RuntimeConfig{
		Config: Config{
			PageObservationMode: PageObservationModeHTTP,
		},
	}, OpenPageRequest{
		URL: server.URL,
	})
	if err != nil {
		t.Fatalf("client.OpenPage: %v", err)
	}

	if resp.Title != "Docs" {
		t.Fatalf("title = %q, want Docs", resp.Title)
	}
	if !strings.Contains(resp.Content, "Ship automation fast.") {
		t.Fatalf("content = %q, want extracted body text", resp.Content)
	}
}

func TestClientOpenPageWithJinaParsesMarkdownSection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer secret-token" {
			t.Fatalf("authorization = %q, want Bearer secret-token", auth)
		}
		_, _ = w.Write([]byte("Title: Example Page\n\nURL Source: https://example.com\n\nMarkdown Content:\n# Example Page\n\nHelpful content"))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	resp, err := client.OpenPage(context.Background(), RuntimeConfig{
		Config: Config{
			PageObservationMode: PageObservationModeJina,
			JinaReaderBaseURL:   server.URL,
		},
		JinaAPIKey: "secret-token",
	}, OpenPageRequest{
		URL:  "https://example.com",
		Mode: PageObservationModeJina,
	})
	if err != nil {
		t.Fatalf("client.OpenPage: %v", err)
	}

	if resp.Title != "Example Page" {
		t.Fatalf("title = %q, want Example Page", resp.Title)
	}
	if !strings.Contains(resp.Content, "Helpful content") {
		t.Fatalf("content = %q, want markdown body", resp.Content)
	}
}
