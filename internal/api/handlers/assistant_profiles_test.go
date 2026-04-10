package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/assistants"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

func TestAssistantProfileHandlerReturnsDefaultsAndPersistsUpdates(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	handler := NewAssistantProfileHandler(assistants.NewStore(query.NewAppConfigStore(database.DB)))
	app := fiber.New()
	app.Get("/assistant-profiles/:scope", handler.Get)
	app.Put("/assistant-profiles/:scope", handler.Update)
	app.Post("/assistant-profiles/:scope/restore-defaults", handler.RestoreDefaults)

	getDefaultRes := performJSONRequest(t, app, http.MethodGet, "/assistant-profiles/pipeline_editor", nil)
	if getDefaultRes.StatusCode != fiber.StatusOK {
		t.Fatalf("default status = %d, want %d", getDefaultRes.StatusCode, fiber.StatusOK)
	}

	var defaultProfile assistantProfileResponse
	if err := json.NewDecoder(getDefaultRes.Body).Decode(&defaultProfile); err != nil {
		t.Fatalf("decode default profile: %v", err)
	}
	if defaultProfile.Scope != "pipeline_editor" {
		t.Fatalf("scope = %q, want %q", defaultProfile.Scope, "pipeline_editor")
	}
	if len(defaultProfile.EnabledModules) == 0 {
		t.Fatal("expected default enabled modules")
	}

	getChatRes := performJSONRequest(t, app, http.MethodGet, "/assistant-profiles/chat_window", nil)
	if getChatRes.StatusCode != fiber.StatusOK {
		t.Fatalf("chat status = %d, want %d", getChatRes.StatusCode, fiber.StatusOK)
	}

	var chatProfile assistantProfileResponse
	if err := json.NewDecoder(getChatRes.Body).Decode(&chatProfile); err != nil {
		t.Fatalf("decode chat profile: %v", err)
	}
	if chatProfile.EnabledModules == nil {
		t.Fatal("expected chat profile enabled modules to be a non-nil slice")
	}
	if len(chatProfile.EnabledModules) != 0 {
		t.Fatalf("chat enabled modules = %+v, want empty slice", chatProfile.EnabledModules)
	}

	updateRes := performJSONRequest(t, app, http.MethodPut, "/assistant-profiles/pipeline_editor", map[string]any{
		"system_instructions": "Use concise answers.",
	})
	if updateRes.StatusCode != fiber.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRes.StatusCode, fiber.StatusOK)
	}

	var updated assistantProfileResponse
	if err := json.NewDecoder(updateRes.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated profile: %v", err)
	}
	if updated.SystemInstructions != "Use concise answers." {
		t.Fatalf("system instructions = %q, want %q", updated.SystemInstructions, "Use concise answers.")
	}
	defaultModules := assistants.DefaultProfile(assistants.ScopePipelineEditor).EnabledModules
	if len(updated.EnabledModules) != len(defaultModules) {
		t.Fatalf("enabled modules = %+v, want %+v", updated.EnabledModules, defaultModules)
	}
	for index := range defaultModules {
		if updated.EnabledModules[index] != defaultModules[index] {
			t.Fatalf("enabled modules = %+v, want %+v", updated.EnabledModules, defaultModules)
		}
	}

	restoreRes := performJSONRequest(t, app, http.MethodPost, "/assistant-profiles/pipeline_editor/restore-defaults", nil)
	if restoreRes.StatusCode != fiber.StatusOK {
		t.Fatalf("restore status = %d, want %d", restoreRes.StatusCode, fiber.StatusOK)
	}

	var restored assistantProfileResponse
	if err := json.NewDecoder(restoreRes.Body).Decode(&restored); err != nil {
		t.Fatalf("decode restored profile: %v", err)
	}
	if restored.SystemInstructions == "Use concise answers." {
		t.Fatal("expected restore defaults to replace updated instructions")
	}
	if len(restored.EnabledModules) == 0 {
		t.Fatal("expected restored default modules")
	}
}
