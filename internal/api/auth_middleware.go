package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
)

const authSessionLocalKey = "auth_session"

func authMiddleware(service *auth.Service, trustProxy bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Method() == http.MethodOptions || isPublicAPIPath(c.Path()) {
			return c.Next()
		}

		if session, ok := service.Session(c.Cookies(service.CookieName())); ok {
			c.Locals(authSessionLocalKey, session)
			return c.Next()
		}

		clearAuthCookie(c, service.CookieName(), trustProxy)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
}

func websocketAuthMiddleware(service *auth.Service, trustProxy bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if session, ok := service.Session(c.Cookies(service.CookieName())); ok {
			c.Locals(authSessionLocalKey, session)
			return c.Next()
		}

		clearAuthCookie(c, service.CookieName(), trustProxy)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
	}
}

func isPublicAPIPath(path string) bool {
	switch normalized := strings.TrimSpace(path); normalized {
	case "/api/v1/health",
		"/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/auth/session",
		"/api/v1/channels/connect":
		return true
	default:
		return false
	}
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
