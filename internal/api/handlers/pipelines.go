package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/automator/internal/db/models"
	"github.com/FlameInTheDark/automator/internal/db/query"
	"github.com/FlameInTheDark/automator/internal/pipelineops"
	"github.com/FlameInTheDark/automator/internal/scheduler"
	"github.com/FlameInTheDark/automator/internal/templateops"
)

type PipelineHandler struct {
	store     *query.PipelineStore
	scheduler *scheduler.Scheduler
}

func NewPipelineHandler(store *query.PipelineStore, scheduler *scheduler.Scheduler) *PipelineHandler {
	return &PipelineHandler{store: store, scheduler: scheduler}
}

func (h *PipelineHandler) List(c *fiber.Ctx) error {
	ctx := c.Context()
	pipelines, err := h.store.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if pipelines == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	return c.JSON(pipelines)
}

func (h *PipelineHandler) Create(c *fiber.Ctx) error {
	var req models.Pipeline
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.Nodes == "" {
		req.Nodes = "[]"
	}
	if req.Edges == "" {
		req.Edges = "[]"
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if err := validatePipelineDefinition(req.Nodes, req.Edges); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	ctx := c.Context()
	if err := h.store.Create(ctx, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if h.scheduler != nil {
		_ = h.scheduler.Reload(ctx)
	}

	return c.Status(fiber.StatusCreated).JSON(req)
}

func (h *PipelineHandler) Get(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	pipeline, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "pipeline not found",
		})
	}

	return c.JSON(pipeline)
}

func (h *PipelineHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	existing, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "pipeline not found",
		})
	}

	var req models.Pipeline
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}
	req.ID = id
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Description == nil {
		req.Description = existing.Description
	}
	if req.Nodes == "" {
		req.Nodes = existing.Nodes
	}
	if req.Edges == "" {
		req.Edges = existing.Edges
	}
	if req.Viewport == nil {
		req.Viewport = existing.Viewport
	}
	if req.Status == "" {
		req.Status = existing.Status
	}
	if err := validatePipelineDefinition(req.Nodes, req.Edges); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := h.store.Update(ctx, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if h.scheduler != nil {
		_ = h.scheduler.Reload(ctx)
	}

	return c.JSON(req)
}

func validatePipelineDefinition(nodesJSON string, edgesJSON string) error {
	return pipelineops.ValidateDefinition(nodesJSON, edgesJSON)
}

func (h *PipelineHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	if err := h.store.Delete(ctx, id); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if h.scheduler != nil {
		_ = h.scheduler.Reload(ctx)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *PipelineHandler) Export(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	pipelineModel, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "pipeline not found",
		})
	}

	document, err := templateops.BuildPipelineDocument(*pipelineModel)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(document)
}
