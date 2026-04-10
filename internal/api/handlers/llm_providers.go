package handlers

import (
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/llm"
)

type LLMProviderHandler struct {
	store *query.LLMProviderStore
}

func NewLLMProviderHandler(store *query.LLMProviderStore) *LLMProviderHandler {
	return &LLMProviderHandler{store: store}
}

func (h *LLMProviderHandler) List(c *fiber.Ctx) error {
	ctx := c.Context()
	providers, err := h.store.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if providers == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	return c.JSON(providers)
}

func (h *LLMProviderHandler) Create(c *fiber.Ctx) error {
	var req models.LLMProvider
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	ctx := c.Context()
	if err := h.store.Create(ctx, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(req)
}

func (h *LLMProviderHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	provider, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "provider not found",
		})
	}

	return c.JSON(provider)
}

func (h *LLMProviderHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")

	var req models.LLMProvider
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}
	req.ID = id

	ctx := c.Context()
	if err := h.store.Update(ctx, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(req)
}

func (h *LLMProviderHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	if err := h.store.Delete(ctx, id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *LLMProviderHandler) ListModels(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	provider, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "provider not found",
		})
	}

	config, err := llm.ConfigFromModel(provider)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid provider configuration: " + err.Error(),
		})
	}

	return listModelsFromConfig(c, config)
}

func (h *LLMProviderHandler) DiscoverModels(c *fiber.Ctx) error {
	var req models.LLMProvider
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	config, err := llm.ConfigFromModel(&req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid provider configuration: " + err.Error(),
		})
	}

	return listModelsFromConfig(c, config)
}

func listModelsFromConfig(c *fiber.Ctx, config llm.Config) error {
	if !llm.SupportsModelDiscovery(config.ProviderType) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("model discovery is not supported for provider type %s", config.ProviderType),
		})
	}

	models, err := llm.ListModels(c.Context(), config)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(models)
}
