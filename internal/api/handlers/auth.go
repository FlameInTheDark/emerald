package handlers

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
)

type AuthHandler struct {
	service    *auth.Service
	trustProxy bool
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authSessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
	IsSuperAdmin  bool   `json:"is_super_admin"`
	ExpiresAt     string `json:"expires_at"`
}

type AuthHandlerOptions struct {
	TrustProxy bool
}

func NewAuthHandler(service *auth.Service, opts ...AuthHandlerOptions) *AuthHandler {
	handler := &AuthHandler{service: service}
	if len(opts) > 0 {
		handler.trustProxy = opts[0].TrustProxy
	}
	return handler
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
		if errors.Is(err, auth.ErrRateLimited) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many login attempts, please retry shortly"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create session"})
	}

	setAuthCookie(c, h.service.CookieName(), token, session.ExpiresAt, h.trustProxy)

	return c.JSON(toAuthSessionResponse(session))
}

func (h *AuthHandler) Session(c *fiber.Ctx) error {
	session, ok := h.service.Session(c.Cookies(h.service.CookieName()))
	if !ok {
		clearAuthCookie(c, h.service.CookieName(), h.trustProxy)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}

	return c.JSON(toAuthSessionResponse(session))
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	h.service.Logout(c.Cookies(h.service.CookieName()))
	clearAuthCookie(c, h.service.CookieName(), h.trustProxy)
	return c.SendStatus(fiber.StatusNoContent)
}

func toAuthSessionResponse(session auth.Session) authSessionResponse {
	return authSessionResponse{
		Authenticated: true,
		Username:      session.Username,
		IsSuperAdmin:  session.IsSuperAdmin,
		ExpiresAt:     session.ExpiresAt.Format(time.RFC3339),
	}
}

func setAuthCookie(c *fiber.Ctx, cookieName string, token string, expiresAt time.Time, trustProxy bool) {
	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   isSecureRequest(c, trustProxy),
		Expires:  expiresAt,
	})
}

func clearAuthCookie(c *fiber.Ctx, cookieName string, trustProxy bool) {
	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		SameSite: "Lax",
		Secure:   isSecureRequest(c, trustProxy),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func isSecureRequest(c *fiber.Ctx, trustProxy bool) bool {
	if strings.EqualFold(c.Protocol(), "https") {
		return true
	}
	if !trustProxy {
		return false
	}

	forwardedProto := strings.ToLower(strings.TrimSpace(c.Get("X-Forwarded-Proto")))
	if forwardedProto == "" {
		return false
	}

	for _, part := range strings.Split(forwardedProto, ",") {
		if strings.TrimSpace(part) == "https" {
			return true
		}
	}
	return false
}
