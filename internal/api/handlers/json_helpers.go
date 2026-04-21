package handlers

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func bodyIncludesJSONField(c *fiber.Ctx, field string) bool {
	if c == nil || len(c.Body()) == 0 {
		return false
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(c.Body(), &raw); err != nil {
		return false
	}
	_, ok := raw[field]
	return ok
}

func defaultString(got string, fallback string) string {
	if strings.TrimSpace(got) == "" {
		return fallback
	}
	return strings.TrimSpace(got)
}
