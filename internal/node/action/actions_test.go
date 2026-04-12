package action

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPActionExecuteReturnsJSONResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	config, err := json.Marshal(httpActionConfig{
		URL:    server.URL,
		Method: http.MethodGet,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := (&HTTPAction{}).Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	response, ok := output["response"].(map[string]any)
	if !ok {
		t.Fatalf("response = %#v, want object", output["response"])
	}
	if got := response["status"]; got != "ok" {
		t.Fatalf("response.status = %#v, want ok", got)
	}
}

func TestHTTPActionExecuteReturnsStructuredXMLResponseBody(t *testing.T) {
	t.Parallel()

	const rss = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<rss version=\"2.0\"><channel><title>Golang Weekly</title><item><title>First</title></item><item><title>Second</title></item></channel></rss>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write([]byte(rss))
	}))
	defer server.Close()

	config, err := json.Marshal(httpActionConfig{
		URL:    server.URL,
		Method: http.MethodGet,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := (&HTTPAction{}).Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	response, ok := output["response"].(map[string]any)
	if !ok {
		t.Fatalf("response = %#v, want object", output["response"])
	}

	root, ok := response["rss"].(map[string]any)
	if !ok {
		t.Fatalf("response.rss = %#v, want object", response["rss"])
	}
	if got := root["@version"]; got != "2.0" {
		t.Fatalf("response.rss.@version = %#v, want 2.0", got)
	}

	channel, ok := root["channel"].(map[string]any)
	if !ok {
		t.Fatalf("response.rss.channel = %#v, want object", root["channel"])
	}
	if got := channel["title"]; got != "Golang Weekly" {
		t.Fatalf("response.rss.channel.title = %#v, want Golang Weekly", got)
	}

	items, ok := channel["item"].([]any)
	if !ok {
		t.Fatalf("response.rss.channel.item = %#v, want array", channel["item"])
	}
	if len(items) != 2 {
		t.Fatalf("len(response.rss.channel.item) = %d, want 2", len(items))
	}

	first, ok := items[0].(map[string]any)
	if !ok || first["title"] != "First" {
		t.Fatalf("first item = %#v, want title First", items[0])
	}
	second, ok := items[1].(map[string]any)
	if !ok || second["title"] != "Second" {
		t.Fatalf("second item = %#v, want title Second", items[1])
	}
}

func TestHTTPActionExecuteReturnsHTMLResponseBodyAsString(t *testing.T) {
	t.Parallel()

	const html = "<html><head><title>Emerald</title></head><body>ok</body></html>"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()

	config, err := json.Marshal(httpActionConfig{
		URL:    server.URL,
		Method: http.MethodGet,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := (&HTTPAction{}).Execute(context.Background(), config, nil)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	response, ok := output["response"].(string)
	if !ok {
		t.Fatalf("response = %#v, want string", output["response"])
	}
	if response != html {
		t.Fatalf("response = %q, want %q", response, html)
	}
}
