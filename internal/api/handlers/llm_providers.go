package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/llm"
)

type LLMProviderHandler struct {
	store         *query.LLMProviderStore
	authService   *auth.Service
	auditLogStore *query.AuditLogStore
}

type LLMProviderHandlerOptions struct {
	AuthService   *auth.Service
	AuditLogStore *query.AuditLogStore
}

type llmProviderRequest struct {
	Name         string  `json:"name"`
	ProviderType string  `json:"provider_type"`
	APIKey       *string `json:"api_key,omitempty"`
	BaseURL      *string `json:"base_url,omitempty"`
	Model        string  `json:"model"`
	Config       *string `json:"config,omitempty"`
	IsDefault    bool    `json:"is_default"`
}

type llmProviderResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProviderType string    `json:"provider_type"`
	APIKey       string    `json:"api_key,omitempty"`
	HasSecret    bool      `json:"has_secret"`
	BaseURL      *string   `json:"base_url,omitempty"`
	Model        string    `json:"model"`
	Config       *string   `json:"config,omitempty"`
	IsDefault    bool      `json:"is_default"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func NewLLMProviderHandler(store *query.LLMProviderStore, opts ...LLMProviderHandlerOptions) *LLMProviderHandler {
	handler := &LLMProviderHandler{store: store}
	if len(opts) > 0 {
		handler.authService = opts[0].AuthService
		handler.auditLogStore = opts[0].AuditLogStore
	}
	return handler
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

	responses := make([]llmProviderResponse, 0, len(providers))
	for _, provider := range providers {
		responses = append(responses, newLLMProviderResponse(provider, false))
	}

	return c.JSON(responses)
}

func (h *LLMProviderHandler) Create(c *fiber.Ctx) error {
	var req llmProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	provider := &models.LLMProvider{
		Name:         strings.TrimSpace(req.Name),
		ProviderType: strings.TrimSpace(req.ProviderType),
		BaseURL:      req.BaseURL,
		Model:        strings.TrimSpace(req.Model),
		Config:       req.Config,
		IsDefault:    req.IsDefault,
	}
	if req.APIKey != nil {
		provider.APIKey = *req.APIKey
	}

	ctx := c.Context()
	if err := h.store.Create(ctx, provider); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	created, err := h.store.GetByID(ctx, provider.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(newLLMProviderResponse(*created, false))
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

	return c.JSON(newLLMProviderResponse(*provider, false))
}

func (h *LLMProviderHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	existing, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "provider not found"})
	}

	var req llmProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	provider := &models.LLMProvider{
		ID:           id,
		Name:         defaultString(req.Name, existing.Name),
		ProviderType: defaultString(req.ProviderType, existing.ProviderType),
		APIKey:       existing.APIKey,
		BaseURL:      existing.BaseURL,
		Model:        defaultString(req.Model, existing.Model),
		Config:       existing.Config,
		IsDefault:    req.IsDefault,
	}
	if !bodyIncludesJSONField(c, "is_default") {
		provider.IsDefault = existing.IsDefault
	}
	if bodyIncludesJSONField(c, "api_key") {
		if req.APIKey != nil {
			provider.APIKey = *req.APIKey
		} else {
			provider.APIKey = ""
		}
	}
	if bodyIncludesJSONField(c, "base_url") {
		provider.BaseURL = req.BaseURL
	}
	if bodyIncludesJSONField(c, "config") {
		provider.Config = req.Config
	}

	if err := h.store.Update(ctx, provider); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	refreshed, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newLLMProviderResponse(*refreshed, false))
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

func (h *LLMProviderHandler) Reveal(c *fiber.Ctx) error {
	session, err := requireRevealAuthorization(c, h.authService)
	if err != nil {
		return c.Status(statusCodeForRevealError(err)).JSON(fiber.Map{"error": err.Error()})
	}

	provider, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "provider not found"})
	}

	if err := writeAuditLog(c, h.auditLogStore, session, "llm_provider.reveal", "llm_provider", provider.ID, map[string]any{"name": provider.Name}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to write audit log"})
	}

	return c.JSON(newLLMProviderResponse(*provider, true))
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
	var req llmProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	provider := &models.LLMProvider{
		Name:         strings.TrimSpace(req.Name),
		ProviderType: strings.TrimSpace(req.ProviderType),
		BaseURL:      req.BaseURL,
		Model:        strings.TrimSpace(req.Model),
		Config:       req.Config,
		IsDefault:    req.IsDefault,
	}
	if req.APIKey != nil {
		provider.APIKey = *req.APIKey
	}

	config, err := llm.ConfigFromModel(provider)
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

func newLLMProviderResponse(provider models.LLMProvider, includeSecret bool) llmProviderResponse {
	response := llmProviderResponse{
		ID:           provider.ID,
		Name:         provider.Name,
		ProviderType: provider.ProviderType,
		HasSecret:    provider.HasSecret || strings.TrimSpace(provider.APIKey) != "",
		BaseURL:      provider.BaseURL,
		Model:        provider.Model,
		Config:       provider.Config,
		IsDefault:    provider.IsDefault,
		CreatedAt:    provider.CreatedAt,
		UpdatedAt:    provider.UpdatedAt,
	}
	if includeSecret {
		response.APIKey = provider.APIKey
	}
	return response
}
