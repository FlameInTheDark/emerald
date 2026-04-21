package query

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestUserStoreCreateHashesPassword(t *testing.T) {
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

	store := NewUserStore(database.DB, nil)
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
		t.Fatal("expected stored password to be hashed")
	}
	if !auth.IsPasswordHash(rawPassword) {
		t.Fatalf("raw password = %q, want argon2id hash", rawPassword)
	}

	user, err := store.GetByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	if user.Password != rawPassword {
		t.Fatalf("user password = %q, want stored hash", user.Password)
	}
	match, err := auth.VerifyPasswordHash(user.Password, "admin")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !match {
		t.Fatal("expected password hash to verify")
	}
}

func TestUserStoreGetByUsernameReturnsNilWhenMissing(t *testing.T) {
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

	store := NewUserStore(database.DB, nil)
	user, err := store.GetByUsername(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user != nil {
		t.Fatalf("user = %#v, want nil", user)
	}
}

func TestUserStoreUpdatePasswordHashesNewValue(t *testing.T) {
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

	store := NewUserStore(database.DB, nil)
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
		t.Fatal("expected updated password to be hashed")
	}
	if !auth.IsPasswordHash(rawPassword) {
		t.Fatalf("raw password = %q, want argon2id hash", rawPassword)
	}

	updatedUser, err := store.GetByUsername(context.Background(), "operator")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if updatedUser == nil {
		t.Fatal("expected user")
	}
	match, err := auth.VerifyPasswordHash(updatedUser.Password, "after")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !match {
		t.Fatal("expected updated password hash to verify")
	}
}
