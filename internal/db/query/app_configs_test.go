package query

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db"
)

func TestAppConfigStoreGetEncryptionKey(t *testing.T) {
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

	store := NewAppConfigStore(database.DB)

	key, ok, err := store.GetEncryptionKey(context.Background())
	if err != nil {
		t.Fatalf("GetEncryptionKey: %v", err)
	}
	if ok {
		t.Fatalf("ok = %v, want false for missing key", ok)
	}
	if key != "" {
		t.Fatalf("key = %q, want empty", key)
	}

	expected := strings.Repeat("k", 32)
	if err := store.Set(context.Background(), AppConfigKeyEncryptionKey, expected); err != nil {
		t.Fatalf("Set: %v", err)
	}

	key, ok, err = store.GetEncryptionKey(context.Background())
	if err != nil {
		t.Fatalf("GetEncryptionKey second call: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if key != expected {
		t.Fatalf("key = %q, want %q", key, expected)
	}
}
