package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	iauth "github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type stubUserStore struct {
	user *models.User
}

func (s *stubUserStore) GetByUsername(_ context.Context, username string) (*models.User, error) {
	if s.user != nil && s.user.Username == username {
		return s.user, nil
	}
	return nil, nil
}

func (s *stubUserStore) UpdatePassword(_ context.Context, id string, password string) error {
	if s.user == nil || s.user.ID != id {
		return nil
	}
	hashed, err := iauth.HashPassword(password)
	if err != nil {
		return err
	}
	s.user.Password = hashed
	return nil
}

func TestAuthMiddlewareProtectsPrivateRoutes(t *testing.T) {
	t.Parallel()

	service := iauth.NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: mustHashPassword(t, "admin")},
	}, iauth.Config{})
	app := fiber.New()
	app.Use("/api/v1", authMiddleware(service, false))
	app.Get("/api/v1/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/api/v1/private", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/private", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestAuthMiddlewareAllowsPublicRoutes(t *testing.T) {
	t.Parallel()

	service := iauth.NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: mustHashPassword(t, "admin")},
	}, iauth.Config{})
	app := fiber.New()
	app.Use("/api/v1", authMiddleware(service, false))
	app.Get("/api/v1/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}
}

func TestAuthMiddlewareAllowsAuthenticatedRoutes(t *testing.T) {
	t.Parallel()

	service := iauth.NewService(&stubUserStore{
		user: &models.User{ID: "user-1", Username: "admin", Password: mustHashPassword(t, "admin")},
	}, iauth.Config{})
	token, _, err := service.Login(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	app := fiber.New()
	app.Use("/api/v1", authMiddleware(service, false))
	app.Get("/api/v1/private", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/private", nil)
	req.AddCookie(&http.Cookie{Name: service.CookieName(), Value: token})
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()

	hashed, err := iauth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return hashed
}
