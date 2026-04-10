package query

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestUserStoreCreateEncryptsPassword(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	appConfigStore := NewAppConfigStore(database.DB)
	key, err := appConfigStore.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	store := NewUserStore(database.DB, encryptor)
	if err := store.Create(context.Background(), &models.User{
		Username: "admin",
		Password: "admin",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var rawPassword string
	if err := database.DB.QueryRowContext(context.Background(), "SELECT password FROM users WHERE username = ?", "admin").Scan(&rawPassword); err != nil {
		t.Fatalf("query raw password: %v", err)
	}
	if rawPassword == "admin" {
		t.Fatal("expected stored password to be encrypted")
	}

	user, err := store.GetByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	if user.Password != "admin" {
		t.Fatalf("decrypted password = %q, want admin", user.Password)
	}
}

func TestUserStoreGetByUsernameReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	appConfigStore := NewAppConfigStore(database.DB)
	key, err := appConfigStore.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	store := NewUserStore(database.DB, encryptor)
	user, err := store.GetByUsername(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user != nil {
		t.Fatalf("user = %#v, want nil", user)
	}
}

func TestUserStoreUpdatePasswordEncryptsNewValue(t *testing.T) {
	t.Parallel()

	database, err := db.New(filepath.Join(t.TempDir(), "emerald.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	appConfigStore := NewAppConfigStore(database.DB)
	key, err := appConfigStore.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	store := NewUserStore(database.DB, encryptor)
	user := &models.User{
		Username: "operator",
		Password: "before",
	}
	if err := store.Create(context.Background(), user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.UpdatePassword(context.Background(), user.ID, "after"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	var rawPassword string
	if err := database.DB.QueryRowContext(context.Background(), "SELECT password FROM users WHERE id = ?", user.ID).Scan(&rawPassword); err != nil {
		t.Fatalf("query raw password: %v", err)
	}
	if rawPassword == "after" {
		t.Fatal("expected updated password to be encrypted")
	}

	updatedUser, err := store.GetByUsername(context.Background(), "operator")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if updatedUser == nil {
		t.Fatal("expected user")
	}
	if updatedUser.Password != "after" {
		t.Fatalf("updated password = %q, want after", updatedUser.Password)
	}
}
