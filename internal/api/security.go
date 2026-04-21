package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	appconfig "github.com/FlameInTheDark/emerald/internal/config"
)

func securityHeadersMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; object-src 'none'; img-src 'self' data: https:; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self' https: ws: wss:; font-src 'self' data:; form-action 'self'")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		return c.Next()
	}
}

func stateChangingOriginMiddleware(cfg appconfig.SecurityConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !requiresOriginValidation(c.Method()) {
			return c.Next()
		}

		origin := normalizeOrigin(c.Get(fiber.HeaderOrigin))
		if origin == "" {
			return c.Next()
		}
		if originAllowed(c, origin, cfg.AllowedOrigins) {
			return c.Next()
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "origin is not allowed"})
	}
}

func websocketOriginMiddleware(cfg appconfig.SecurityConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		origin := normalizeOrigin(c.Get(fiber.HeaderOrigin))
		if origin == "" || originAllowed(c, origin, cfg.AllowedOrigins) {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "origin is not allowed"})
	}
}

func requiresOriginValidation(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func originAllowed(c *fiber.Ctx, origin string, allowedOrigins []string) bool {
	if origin == "" {
		return true
	}
	if origin == normalizeOrigin(c.BaseURL()) {
		return true
	}
	for _, candidate := range allowedOrigins {
		if origin == normalizeOrigin(candidate) {
			return true
		}
	}
	return false
}

func normalizeOrigin(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(strings.ToLower(trimmed), "/")
	}

	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}
