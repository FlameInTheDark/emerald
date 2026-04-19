package webtools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

func TestStoreResolveUsesEncryptedJinaSecretAndMarksSearchReady(t *testing.T) {
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

	appConfigStore := query.NewAppConfigStore(database.DB)
	secretStore := query.NewSecretStore(database.DB, encryptor)
	if err := secretStore.Create(context.Background(), &models.Secret{
		Name:  "jina_api_key",
		Value: "jina-secret-token",
	}); err != nil {
		t.Fatalf("secretStore.Create: %v", err)
	}

	store := NewStore(appConfigStore, secretStore)
	saved, err := store.Set(context.Background(), Config{
		SearchProvider:       SearchProviderJina,
		PageObservationMode:  PageObservationModeJina,
		JinaAPIKeySecretName: "jina_api_key",
	})
	if err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	if !saved.SearchReady {
		t.Fatal("expected Jina search to be ready when the API key secret exists")
	}
	if !saved.PageReadReady {
		t.Fatal("expected Jina page reading to be ready")
	}

	resolved, err := store.Resolve(context.Background())
	if err != nil {
		t.Fatalf("store.Resolve: %v", err)
	}
	if resolved.JinaAPIKey != "jina-secret-token" {
		t.Fatalf("resolved.JinaAPIKey = %q, want %q", resolved.JinaAPIKey, "jina-secret-token")
	}
}

func TestStoreWarnsWhenJinaSearchHasNoSecret(t *testing.T) {
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

	store := NewStore(query.NewAppConfigStore(database.DB), nil)
	cfg, err := store.Set(context.Background(), Config{
		SearchProvider:      SearchProviderJina,
		PageObservationMode: PageObservationModeHTTP,
	})
	if err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	if cfg.SearchReady {
		t.Fatal("expected Jina search without a secret to be marked not ready")
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("expected a warning for missing Jina API key secret")
	}
}
