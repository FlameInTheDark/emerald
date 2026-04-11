package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/nodedefs"
	"github.com/FlameInTheDark/emerald/internal/plugins"
)

func TestNodeDefinitionsHandlerRefreshReturnsPayloadEvenOnRefreshError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	notDirectoryPath := filepath.Join(root, "plugins.json")
	if err := os.WriteFile(notDirectoryPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	handler := NewNodeDefinitionsHandler(nodedefs.NewService(plugins.NewManager(notDirectoryPath)))
	app := fiber.New()
	app.Post("/node-definitions/refresh", handler.Refresh)

	req := httptest.NewRequest("POST", "/node-definitions/refresh", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var body nodeDefinitionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Error == "" {
		t.Fatalf("expected refresh error to be included in response")
	}
	if len(body.Definitions) == 0 {
		t.Fatalf("expected builtin definitions to still be returned")
	}
}
