package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
	"github.com/FlameInTheDark/emerald/internal/scheduler"
	"github.com/FlameInTheDark/emerald/internal/templateops"
)

type PipelineHandler struct {
	store     *query.PipelineStore
	scheduler *scheduler.Scheduler
	validator definitionValidator
}

type definitionValidator interface {
	ValidateDefinition(ctx context.Context, nodesJSON string, edgesJSON string, allowUnavailablePlugins bool) error
}

func NewPipelineHandler(store *query.PipelineStore, scheduler *scheduler.Scheduler, validators ...definitionValidator) *PipelineHandler {
	handler := &PipelineHandler{store: store, scheduler: scheduler}
	if len(validators) > 0 {
		handler.validator = validators[0]
	}
	return handler
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
	ctx := c.Context()

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
	if err := h.validatePipelineDefinition(ctx, req.Nodes, req.Edges, req.Status != pipelineops.StatusActive); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

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
	if err := h.validatePipelineDefinition(ctx, req.Nodes, req.Edges, req.Status != pipelineops.StatusActive); err != nil {
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

func (h *PipelineHandler) validatePipelineDefinition(ctx context.Context, nodesJSON string, edgesJSON string, allowUnavailablePlugins bool) error {
	if err := pipelineops.ValidateDefinition(nodesJSON, edgesJSON); err != nil {
		return err
	}
	if h.validator != nil {
		return h.validator.ValidateDefinition(ctx, nodesJSON, edgesJSON, allowUnavailablePlugins)
	}
	return nil
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
