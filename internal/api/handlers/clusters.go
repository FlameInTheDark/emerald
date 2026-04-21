package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
)

type ClusterHandler struct {
	store         *query.ClusterStore
	authService   *auth.Service
	auditLogStore *query.AuditLogStore
}

type ClusterHandlerOptions struct {
	AuthService   *auth.Service
	AuditLogStore *query.AuditLogStore
}

type clusterUpsertRequest struct {
	Name           string  `json:"name"`
	Host           string  `json:"host"`
	Port           int     `json:"port"`
	APITokenID     string  `json:"api_token_id"`
	APITokenSecret *string `json:"api_token_secret,omitempty"`
	SkipTLSVerify  bool    `json:"skip_tls_verify"`
}

type clusterResponse struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	APITokenID     string    `json:"api_token_id"`
	APITokenSecret string    `json:"api_token_secret,omitempty"`
	HasSecret      bool      `json:"has_secret"`
	SkipTLSVerify  bool      `json:"skip_tls_verify"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func NewClusterHandler(store *query.ClusterStore, opts ...ClusterHandlerOptions) *ClusterHandler {
	handler := &ClusterHandler{store: store}
	if len(opts) > 0 {
		handler.authService = opts[0].AuthService
		handler.auditLogStore = opts[0].AuditLogStore
	}
	return handler
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

	responses := make([]clusterResponse, 0, len(clusters))
	for _, cluster := range clusters {
		responses = append(responses, newClusterResponse(cluster, false))
	}

	return c.JSON(responses)
}

func (h *ClusterHandler) Create(c *fiber.Ctx) error {
	var req clusterUpsertRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}
	if req.APITokenSecret == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "api_token_secret is required"})
	}

	cluster := &models.Cluster{
		Name:           strings.TrimSpace(req.Name),
		Host:           strings.TrimSpace(req.Host),
		Port:           req.Port,
		APITokenID:     strings.TrimSpace(req.APITokenID),
		APITokenSecret: *req.APITokenSecret,
		SkipTLSVerify:  req.SkipTLSVerify,
	}

	ctx := c.Context()
	if err := h.store.Create(ctx, cluster); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	created, err := h.store.GetByID(ctx, cluster.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(newClusterResponse(*created, false))
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

	return c.JSON(newClusterResponse(*cluster, false))
}

func (h *ClusterHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	ctx := c.Context()

	existing, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "cluster not found"})
	}

	var req clusterUpsertRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	updated := &models.Cluster{
		ID:             id,
		Name:           defaultString(req.Name, existing.Name),
		Host:           defaultString(req.Host, existing.Host),
		Port:           req.Port,
		APITokenID:     defaultString(req.APITokenID, existing.APITokenID),
		APITokenSecret: existing.APITokenSecret,
		SkipTLSVerify:  req.SkipTLSVerify,
	}
	if updated.Port == 0 {
		updated.Port = existing.Port
	}
	if bodyIncludesJSONField(c, "api_token_secret") {
		if req.APITokenSecret != nil {
			updated.APITokenSecret = *req.APITokenSecret
		} else {
			updated.APITokenSecret = ""
		}
	}

	if err := h.store.Update(ctx, updated); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	refreshed, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(newClusterResponse(*refreshed, false))
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

func (h *ClusterHandler) Reveal(c *fiber.Ctx) error {
	session, err := requireRevealAuthorization(c, h.authService)
	if err != nil {
		return c.Status(statusCodeForRevealError(err)).JSON(fiber.Map{"error": err.Error()})
	}

	cluster, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "cluster not found"})
	}

	if err := writeAuditLog(c, h.auditLogStore, session, "cluster.reveal", "cluster", cluster.ID, map[string]any{"name": cluster.Name}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to write audit log"})
	}

	return c.JSON(newClusterResponse(*cluster, true))
}

func newClusterResponse(cluster models.Cluster, includeSecret bool) clusterResponse {
	response := clusterResponse{
		ID:            cluster.ID,
		Name:          cluster.Name,
		Host:          cluster.Host,
		Port:          cluster.Port,
		APITokenID:    cluster.APITokenID,
		HasSecret:     cluster.HasSecret || strings.TrimSpace(cluster.APITokenSecret) != "",
		SkipTLSVerify: cluster.SkipTLSVerify,
		CreatedAt:     cluster.CreatedAt,
		UpdatedAt:     cluster.UpdatedAt,
	}
	if includeSecret {
		response.APITokenSecret = cluster.APITokenSecret
	}
	return response
}
