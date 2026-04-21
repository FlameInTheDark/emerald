package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/crypto"
	"github.com/FlameInTheDark/emerald/internal/db"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

func TestUserHandlerChangePassword(t *testing.T) {
	t.Parallel()

	userStore, authService := newUserHandlerTestDeps(t)
	user := &models.User{Username: "admin", Password: "admin"}
	if err := userStore.Create(context.Background(), user); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token, _, err := authService.Login(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	handler := NewUserHandler(userStore, authService)
	app := fiber.New()
	app.Post("/users/change-password", handler.ChangePassword)

	body := bytes.NewBufferString(`{"current_password":"admin","new_password":"new-secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/users/change-password", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authService.CookieName(), Value: token})

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}

	if _, ok := authService.Session(token); ok {
		t.Fatal("expected previous session token to be revoked")
	}

	updatedUser, err := userStore.GetByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if updatedUser == nil {
		t.Fatal("expected updated user")
	}
	if updatedUser.Password == "new-secret" {
		t.Fatal("expected stored password to be hashed")
	}
	if !auth.IsPasswordHash(updatedUser.Password) {
		t.Fatalf("password = %q, want argon2id hash", updatedUser.Password)
	}
	match, err := auth.VerifyPasswordHash(updatedUser.Password, "new-secret")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !match {
		t.Fatal("expected updated password hash to verify")
	}

	if _, _, err := authService.Login(context.Background(), "admin", "admin"); err == nil {
		t.Fatal("expected old password login to fail")
	}
	if _, _, err := authService.Login(context.Background(), "admin", "new-secret"); err != nil {
		t.Fatalf("new password login failed: %v", err)
	}

	var payload authSessionResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Authenticated || payload.Username != "admin" {
		t.Fatalf("unexpected response payload: %+v", payload)
	}
}

func TestUserHandlerChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	t.Parallel()

	userStore, authService := newUserHandlerTestDeps(t)
	if err := userStore.Create(context.Background(), &models.User{Username: "admin", Password: "admin"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	token, _, err := authService.Login(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	handler := NewUserHandler(userStore, authService)
	app := fiber.New()
	app.Post("/users/change-password", handler.ChangePassword)

	body := bytes.NewBufferString(`{"current_password":"wrong","new_password":"new-secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/users/change-password", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: authService.CookieName(), Value: token})

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusUnauthorized)
	}

	user, err := userStore.GetByUsername(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	match, err := auth.VerifyPasswordHash(user.Password, "admin")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !match {
		t.Fatal("expected original password hash to remain valid")
	}
}

func newUserHandlerTestDeps(t *testing.T) (*query.UserStore, *auth.Service) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "emerald.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	})

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	key := strings.Repeat("k", 32)

	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	userStore := query.NewUserStore(database.DB, encryptor)
	authService := auth.NewService(userStore, auth.Config{})

	return userStore, authService
}
