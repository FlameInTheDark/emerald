package handlers

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/automator/internal/templateops"
)

type TemplateHandler struct {
	service *templateops.Service
}

type createTemplateRequest struct {
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Definition  templateops.Definition `json:"definition"`
}

func NewTemplateHandler(service *templateops.Service) *TemplateHandler {
	return &TemplateHandler{service: service}
}

func (h *TemplateHandler) List(c *fiber.Ctx) error {
	templates, err := h.service.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if templates == nil {
		return c.JSON([]templateops.TemplateSummary{})
	}

	return c.JSON(templates)
}

func (h *TemplateHandler) Create(c *fiber.Ctx) error {
	var req createTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	template, err := h.service.Create(c.Context(), templateops.CreateTemplateInput{
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Definition:  req.Definition,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(template)
}

func (h *TemplateHandler) Get(c *fiber.Ctx) error {
	template, err := h.service.Get(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if template == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "template not found"})
	}

	return c.JSON(template)
}

func (h *TemplateHandler) Clone(c *fiber.Ctx) error {
	template, err := h.service.Clone(c.Context(), c.Params("id"))
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(template)
}

func (h *TemplateHandler) Delete(c *fiber.Ctx) error {
	if err := h.service.Delete(c.Context(), c.Params("id")); err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TemplateHandler) CreatePipeline(c *fiber.Ctx) error {
	pipelineModel, err := h.service.CreatePipeline(c.Context(), c.Params("id"))
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(pipelineModel)
}

func (h *TemplateHandler) Export(c *fiber.Ctx) error {
	document, err := h.service.ExportTemplate(c.Context(), c.Params("id"))
	if err != nil {
		status := fiber.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = fiber.StatusNotFound
		}
		return c.Status(status).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(document)
}

func (h *TemplateHandler) ExportAll(c *fiber.Ctx) error {
	document, err := h.service.ExportBundle(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(document)
}

func (h *TemplateHandler) Import(c *fiber.Ctx) error {
	raw := c.Body()
	if len(raw) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "request body is required"})
	}
	if !json.Valid(raw) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "request body must be valid JSON"})
	}

	result, err := h.service.Import(c.Context(), raw)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}
