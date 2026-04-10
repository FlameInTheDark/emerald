package handlers

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type SecretHandler struct {
	store *query.SecretStore
}

type secretUpsertRequest struct {
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

func NewSecretHandler(store *query.SecretStore) *SecretHandler {
	return &SecretHandler{store: store}
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

	return c.JSON(secrets)
}

func (h *SecretHandler) Get(c *fiber.Ctx) error {
	secret, err := h.loadSecret(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(secret)
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

	return c.Status(fiber.StatusCreated).JSON(created)
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

	return c.JSON(refreshed)
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
