package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
)

type AuthHandler struct {
	service *auth.Service
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authSessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
	ExpiresAt     string `json:"expires_at"`
}

func NewAuthHandler(service *auth.Service) *AuthHandler {
	return &AuthHandler{service: service}
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	token, session, err := h.service.Login(c.UserContext(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create session"})
	}

	setAuthCookie(c, h.service.CookieName(), token, session.ExpiresAt)

	return c.JSON(toAuthSessionResponse(session))
}

func (h *AuthHandler) Session(c *fiber.Ctx) error {
	session, ok := h.service.Session(c.Cookies(h.service.CookieName()))
	if !ok {
		clearAuthCookie(c, h.service.CookieName())
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	return c.JSON(toAuthSessionResponse(session))
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	h.service.Logout(c.Cookies(h.service.CookieName()))
	clearAuthCookie(c, h.service.CookieName())
	return c.SendStatus(fiber.StatusNoContent)
}

func toAuthSessionResponse(session auth.Session) authSessionResponse {
	return authSessionResponse{
		Authenticated: true,
		Username:      session.Username,
		ExpiresAt:     session.ExpiresAt.Format(time.RFC3339),
	}
}

func setAuthCookie(c *fiber.Ctx, cookieName string, token string, expiresAt time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   strings.EqualFold(c.Protocol(), "https"),
		Expires:  expiresAt,
	})
}

func clearAuthCookie(c *fiber.Ctx, cookieName string) {
	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}
