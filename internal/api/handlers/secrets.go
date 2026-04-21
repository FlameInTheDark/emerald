package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type SecretHandler struct {
	store         *query.SecretStore
	authService   *auth.Service
	auditLogStore *query.AuditLogStore
}

type secretUpsertRequest struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

func NewSecretHandler(store *query.SecretStore) *SecretHandler {
	return &SecretHandler{store: store}
}

type SecretHandlerOptions struct {
	AuthService   *auth.Service
	AuditLogStore *query.AuditLogStore
}

func NewSecretHandlerWithOptions(store *query.SecretStore, opts SecretHandlerOptions) *SecretHandler {
	return &SecretHandler{
		store:         store,
		authService:   opts.AuthService,
		auditLogStore: opts.AuditLogStore,
	}
}

func (h *SecretHandler) List(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.JSON([]models.Secret{})
	}

	secrets, err := h.store.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if secrets == nil {
		return c.JSON([]models.Secret{})
	}

	responses := make([]secretResponse, 0, len(secrets))
	for _, secret := range secrets {
		responses = append(responses, newSecretResponse(secret, ""))
	}
	return c.JSON(responses)
}

func (h *SecretHandler) Get(c *fiber.Ctx) error {
	secret, err := h.loadSecret(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(newSecretResponse(*secret, ""))
}

func (h *SecretHandler) Create(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "secret store is not configured"})
	}

	var req secretUpsertRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Value == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "value is required"})
	}

	secret := &models.Secret{
		Name:  strings.TrimSpace(req.Name),
		Value: *req.Value,
	}
	if err := h.store.Create(c.Context(), secret); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	created, err := h.loadSecret(c.Context(), secret.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(newSecretResponse(*created, ""))
}

func (h *SecretHandler) Update(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "secret store is not configured"})
	}

	existing, err := h.loadSecret(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var req secretUpsertRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	updated := &models.Secret{
		ID:   existing.ID,
		Name: strings.TrimSpace(req.Name),
	}
	if updated.Name == "" {
		updated.Name = existing.Name
	}
	if req.Value != nil {
		updated.Value = *req.Value
	}

	if err := h.store.Update(c.Context(), updated, req.Value != nil); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	refreshed, err := h.loadSecret(c.Context(), existing.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newSecretResponse(*refreshed, ""))
}

func (h *SecretHandler) Delete(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "secret store is not configured"})
	}

	if _, err := h.loadSecret(c.Context(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	if err := h.store.Delete(c.Context(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *SecretHandler) Reveal(c *fiber.Ctx) error {
	if h == nil || h.store == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "secret store is not configured"})
	}
	session, err := requireRevealAuthorization(c, h.authService)
	if err != nil {
		return c.Status(statusCodeForRevealError(err)).JSON(fiber.Map{"error": err.Error()})
	}

	secret, err := h.loadSecret(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	value, ok, err := h.store.GetValueByID(c.Context(), secret.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "secret not found"})
	}

	if err := writeAuditLog(c, h.auditLogStore, session, "secret.reveal", "secret", secret.ID, map[string]any{"name": secret.Name}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to write audit log"})
	}

	return c.JSON(newSecretResponse(*secret, value))
}

func (h *SecretHandler) loadSecret(ctx context.Context, id string) (*models.Secret, error) {
	if h == nil || h.store == nil {
		return nil, fiber.ErrServiceUnavailable
	}

	secret, err := h.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, fiber.ErrNotFound
	}
	return secret, nil
}

type secretResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	HasValue  bool      `json:"has_value"`
	Value     string    `json:"value,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newSecretResponse(secret models.Secret, value string) secretResponse {
	response := secretResponse{
		ID:        secret.ID,
		Name:      secret.Name,
		HasValue:  secret.HasValue || value != "",
		CreatedAt: secret.CreatedAt,
		UpdatedAt: secret.UpdatedAt,
	}
	if value != "" {
		response.Value = value
	}
	return response
}

func statusCodeForRevealError(err error) int {
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return fiberErr.Code
	}
	return fiber.StatusInternalServerError
}
