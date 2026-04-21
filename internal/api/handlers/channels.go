package handlers

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/channels"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type ChannelHandler struct {
	store         *query.ChannelStore
	contactStore  *query.ChannelContactStore
	service       *channels.Service
	authService   *auth.Service
	auditLogStore *query.AuditLogStore
}

type ChannelHandlerOptions struct {
	AuthService   *auth.Service
	AuditLogStore *query.AuditLogStore
}

type channelResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Config         *string   `json:"config,omitempty"`
	HasSecret      bool      `json:"has_secret"`
	WelcomeMessage string    `json:"welcome_message"`
	ConnectURL     *string   `json:"connect_url,omitempty"`
	Enabled        bool      `json:"enabled"`
	State          *string   `json:"state,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewChannelHandler(store *query.ChannelStore, contactStore *query.ChannelContactStore, service *channels.Service, opts ...ChannelHandlerOptions) *ChannelHandler {
	handler := &ChannelHandler{
		store:        store,
		contactStore: contactStore,
		service:      service,
	}
	if len(opts) > 0 {
		handler.authService = opts[0].AuthService
		handler.auditLogStore = opts[0].AuditLogStore
	}
	return handler
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

	responses := make([]channelResponse, 0, len(channelList))
	for _, channel := range channelList {
		responses = append(responses, newChannelResponse(channel, false))
	}

	return c.JSON(responses)
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

	created, err := h.store.GetByID(c.Context(), req.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(newChannelResponse(*created, false))
}

func (h *ChannelHandler) Get(c *fiber.Ctx) error {
	channel, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "channel not found"})
	}

	return c.JSON(newChannelResponse(*channel, false))
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

	refreshed, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newChannelResponse(*refreshed, false))
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

func (h *ChannelHandler) Reveal(c *fiber.Ctx) error {
	session, err := requireRevealAuthorization(c, h.authService)
	if err != nil {
		return c.Status(statusCodeForRevealError(err)).JSON(fiber.Map{"error": err.Error()})
	}

	channel, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "channel not found"})
	}

	if err := writeAuditLog(c, h.auditLogStore, session, "channel.reveal", "channel", channel.ID, map[string]any{"name": channel.Name}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to write audit log"})
	}

	return c.JSON(newChannelResponse(*channel, true))
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

func newChannelResponse(channel models.Channel, includeSecret bool) channelResponse {
	response := channelResponse{
		ID:             channel.ID,
		Name:           channel.Name,
		Type:           channel.Type,
		HasSecret:      channel.HasSecret || (channel.Config != nil && *channel.Config != ""),
		WelcomeMessage: channel.WelcomeMessage,
		ConnectURL:     channel.ConnectURL,
		Enabled:        channel.Enabled,
		State:          channel.State,
		CreatedAt:      channel.CreatedAt,
		UpdatedAt:      channel.UpdatedAt,
	}
	if includeSecret {
		response.Config = channel.Config
	}
	return response
}
