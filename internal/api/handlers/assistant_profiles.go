package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/automator/internal/assistants"
)

type AssistantProfileHandler struct {
	store *assistants.Store
}

type assistantProfileUpdateRequest struct {
	SystemInstructions string   `json:"system_instructions"`
	EnabledModules     []string `json:"enabled_modules"`
}

type assistantProfileResponse struct {
	Scope              string   `json:"scope"`
	SystemInstructions string   `json:"system_instructions"`
	EnabledModules     []string `json:"enabled_modules"`
	CreatedAt          string   `json:"created_at,omitempty"`
	UpdatedAt          string   `json:"updated_at,omitempty"`
}

func NewAssistantProfileHandler(store *assistants.Store) *AssistantProfileHandler {
	return &AssistantProfileHandler{store: store}
}

func (h *AssistantProfileHandler) Get(c *fiber.Ctx) error {
	scope := assistants.Scope(strings.TrimSpace(c.Params("scope")))
	profile, err := h.store.Get(c.Context(), scope)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newAssistantProfileResponse(profile))
}

func (h *AssistantProfileHandler) Update(c *fiber.Ctx) error {
	scope := assistants.Scope(strings.TrimSpace(c.Params("scope")))

	var req assistantProfileUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	profile, err := h.store.Set(c.Context(), scope, assistants.Profile{
		Scope:              scope,
		SystemInstructions: req.SystemInstructions,
		EnabledModules:     req.EnabledModules,
	})
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(err.Error(), "not configured") {
			status = fiber.StatusInternalServerError
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newAssistantProfileResponse(profile))
}

func (h *AssistantProfileHandler) RestoreDefaults(c *fiber.Ctx) error {
	scope := assistants.Scope(strings.TrimSpace(c.Params("scope")))
	profile, err := h.store.RestoreDefaults(c.Context(), scope)
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(err.Error(), "not configured") {
			status = fiber.StatusInternalServerError
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newAssistantProfileResponse(profile))
}

func newAssistantProfileResponse(profile assistants.Profile) assistantProfileResponse {
	enabledModules := profile.EnabledModules
	if enabledModules == nil {
		enabledModules = []string{}
	}

	response := assistantProfileResponse{
		Scope:              string(profile.Scope),
		SystemInstructions: profile.SystemInstructions,
		EnabledModules:     enabledModules,
	}
	if profile.CreatedAt != nil {
		response.CreatedAt = profile.CreatedAt.Format(llmChatTimeLayout)
	}
	if profile.UpdatedAt != nil {
		response.UpdatedAt = profile.UpdatedAt.Format(llmChatTimeLayout)
	}
	return response
}
