package query

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db"
)

func TestAppConfigStoreEnsureEncryptionKey(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	store := NewAppConfigStore(database.DB)

	key, err := store.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}

	second, err := store.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey second call: %v", err)
	}
	if second != key {
		t.Fatalf("second key = %q, want %q", second, key)
	}
}
