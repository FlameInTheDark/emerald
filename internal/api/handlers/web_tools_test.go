package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/webtools"
)

func TestWebToolsHandlerReturnsDefaultsAndPersistsUpdates(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("crypto.NewEncryptor: %v", err)
	}

	secretStore := query.NewSecretStore(database.DB, encryptor)
	if err := secretStore.Create(context.Background(), &models.Secret{
		Name:  "jina_api_key",
		Value: "secret-token",
	}); err != nil {
		t.Fatalf("secretStore.Create: %v", err)
	}

	handler := NewWebToolsHandler(webtools.NewStore(query.NewAppConfigStore(database.DB), secretStore))
	app := fiber.New()
	app.Get("/web-tools/config", handler.Get)
	app.Put("/web-tools/config", handler.Update)

	defaultRes := performJSONRequest(t, app, http.MethodGet, "/web-tools/config", nil)
	if defaultRes.StatusCode != fiber.StatusOK {
		t.Fatalf("default status = %d, want %d", defaultRes.StatusCode, fiber.StatusOK)
	}

	var defaultConfig webtools.Config
	if err := json.NewDecoder(defaultRes.Body).Decode(&defaultConfig); err != nil {
		t.Fatalf("decode default config: %v", err)
	}
	if defaultConfig.SearchProvider != webtools.SearchProviderDisabled {
		t.Fatalf("search provider = %q, want disabled", defaultConfig.SearchProvider)
	}
	if defaultConfig.PageObservationMode != webtools.PageObservationModeHTTP {
		t.Fatalf("page mode = %q, want http", defaultConfig.PageObservationMode)
	}

	updateRes := performJSONRequest(t, app, http.MethodPut, "/web-tools/config", map[string]any{
		"search_provider":          "jina",
		"page_observation_mode":    "jina",
		"jina_api_key_secret_name": "jina_api_key",
	})
	if updateRes.StatusCode != fiber.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRes.StatusCode, fiber.StatusOK)
	}

	var updated webtools.Config
	if err := json.NewDecoder(updateRes.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated config: %v", err)
	}
	if !updated.SearchReady {
		t.Fatal("expected Jina search to be ready after selecting a valid secret")
	}
	if !updated.PageReadReady {
		t.Fatal("expected Jina page reading to be ready")
	}
}
