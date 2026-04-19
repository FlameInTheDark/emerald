package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/webtools"
)

type WebToolsHandler struct {
	store *webtools.Store
}

type webToolsUpdateRequest struct {
	SearchProvider       webtools.SearchProvider      `json:"search_provider"`
	PageObservationMode  webtools.PageObservationMode `json:"page_observation_mode"`
	SearXNGBaseURL       string                       `json:"searxng_base_url"`
	JinaSearchBaseURL    string                       `json:"jina_search_base_url"`
	JinaReaderBaseURL    string                       `json:"jina_reader_base_url"`
	JinaAPIKeySecretName string                       `json:"jina_api_key_secret_name"`
}

func NewWebToolsHandler(store *webtools.Store) *WebToolsHandler {
	return &WebToolsHandler{store: store}
}

func (h *WebToolsHandler) Get(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "web tools config store is not configured"})
	}

	cfg, err := h.store.Get(c.Context())
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(err.Error(), "not configured") {
			status = fiber.StatusInternalServerError
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(cfg)
}

func (h *WebToolsHandler) Update(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "web tools config store is not configured"})
	}

	var req webToolsUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	cfg, err := h.store.Set(c.Context(), webtools.Config{
		SearchProvider:       req.SearchProvider,
		PageObservationMode:  req.PageObservationMode,
		SearXNGBaseURL:       req.SearXNGBaseURL,
		JinaSearchBaseURL:    req.JinaSearchBaseURL,
		JinaReaderBaseURL:    req.JinaReaderBaseURL,
		JinaAPIKeySecretName: req.JinaAPIKeySecretName,
	})
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(err.Error(), "not configured") {
			status = fiber.StatusInternalServerError
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(cfg)
}
