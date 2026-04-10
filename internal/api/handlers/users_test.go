package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	if updatedUser.Password != "new-secret" {
		t.Fatalf("password = %q, want new-secret", updatedUser.Password)
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
	if user.Password != "admin" {
		t.Fatalf("password = %q, want admin", user.Password)
	}
}

func newUserHandlerTestDeps(t *testing.T) (*query.UserStore, *auth.Service) {
	t.Helper()

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

	appConfigStore := query.NewAppConfigStore(database.DB)
	key, err := appConfigStore.EnsureEncryptionKey(context.Background(), "")
	if err != nil {
		t.Fatalf("EnsureEncryptionKey: %v", err)
	}

	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	userStore := query.NewUserStore(database.DB, encryptor)
	authService := auth.NewService(userStore, auth.Config{})

	return userStore, authService
}
