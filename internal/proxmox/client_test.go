package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestClient_UsesEqualsInTokenAuthorizationHeader(t *testing.T) {
	var gotAuth string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{},
		})
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse(server.URL) error = %v", err)
	}

	host := parsedURL.Hostname()
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("Atoi(parsedURL.Port()) error = %v", err)
	}

	client := NewClient(ClientConfig{
		Host:          host,
		Port:          port,
		TokenID:       "root@pam!emerald",
		TokenSecret:   "secret-value",
		SkipTLSVerify: true,
	})

	_, err = client.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}

	want := "PVEAPIToken=root@pam!emerald=secret-value"
	if gotAuth != want {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, want)
	}
}
