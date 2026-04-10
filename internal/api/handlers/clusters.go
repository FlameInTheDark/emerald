package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type ClusterHandler struct {
	store *query.ClusterStore
}

func NewClusterHandler(store *query.ClusterStore) *ClusterHandler {
	return &ClusterHandler{store: store}
}

func (h *ClusterHandler) List(c *fiber.Ctx) error {
	ctx := c.Context()
	clusters, err := h.store.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if clusters == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	return c.JSON(clusters)
}

func (h *ClusterHandler) Create(c *fiber.Ctx) error {
	var req models.Cluster
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

func (h *ClusterHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	cluster, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "cluster not found",
		})
	}

	return c.JSON(cluster)
}

func (h *ClusterHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")

	var req models.Cluster
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

func (h *ClusterHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	if err := h.store.Delete(ctx, id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
