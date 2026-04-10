package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/llm"
)

func llmErrorStatus(err error) int {
	if status, ok := llm.ErrorStatusCode(err); ok {
		switch {
		case status == fiber.StatusTooManyRequests:
			return fiber.StatusTooManyRequests
		case status >= 400 && status < 600:
			return status
		}
	}

	return fiber.StatusInternalServerError
}

func llmErrorMessage(prefix string, err error) string {
	message := strings.TrimSpace(err.Error())
	if prefix == "" {
		return message
	}
	if message == "" {
		return prefix
	}
	return prefix + ": " + message
}
