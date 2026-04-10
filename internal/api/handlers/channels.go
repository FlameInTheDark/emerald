package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/channels"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type ChannelHandler struct {
	store        *query.ChannelStore
	contactStore *query.ChannelContactStore
	service      *channels.Service
}

func NewChannelHandler(store *query.ChannelStore, contactStore *query.ChannelContactStore, service *channels.Service) *ChannelHandler {
	return &ChannelHandler{
		store:        store,
		contactStore: contactStore,
		service:      service,
	}
}

func (h *ChannelHandler) List(c *fiber.Ctx) error {
	channelList, err := h.store.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if channelList == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	return c.JSON(channelList)
}

func (h *ChannelHandler) Create(c *fiber.Ctx) error {
	var req models.Channel
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Config == nil {
		empty := "{}"
		req.Config = &empty
	}

	if err := h.store.Create(c.Context(), &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if h.service != nil {
		_ = h.service.Reload(c.Context())
	}

	return c.Status(fiber.StatusCreated).JSON(req)
}

func (h *ChannelHandler) Get(c *fiber.Ctx) error {
	channel, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "channel not found"})
	}

	return c.JSON(channel)
}

func (h *ChannelHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	existing, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "channel not found"})
	}

	var req models.Channel
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	req.ID = id
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Type == "" {
		req.Type = existing.Type
	}
	if req.Config == nil {
		req.Config = existing.Config
	}
	if req.WelcomeMessage == "" {
		req.WelcomeMessage = existing.WelcomeMessage
	}
	if req.ConnectURL == nil {
		req.ConnectURL = existing.ConnectURL
	}
	if req.State == nil {
		req.State = existing.State
	}
	if !req.Enabled && existing.Enabled && c.Body() != nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(c.Body(), &raw); err != nil || raw["enabled"] == nil {
			req.Enabled = existing.Enabled
		}
	}

	if err := h.store.Update(ctx, &req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if h.service != nil {
		_ = h.service.Reload(ctx)
	}

	return c.JSON(req)
}

func (h *ChannelHandler) Delete(c *fiber.Ctx) error {
	if err := h.store.Delete(c.Context(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if h.service != nil {
		_ = h.service.Reload(c.Context())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ChannelHandler) ListContacts(c *fiber.Ctx) error {
	contacts, err := h.contactStore.ListByChannel(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if contacts == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	return c.JSON(contacts)
}

func (h *ChannelHandler) Connect(c *fiber.Ctx) error {
	var req struct {
		ChannelID string `json:"channel_id,omitempty"`
		Code      string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if h.service == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "channel service is not configured"})
	}

	contact, err := h.service.ConnectContact(c.Context(), req.ChannelID, req.Code)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(contact)
}
