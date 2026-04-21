package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	ik8s "github.com/FlameInTheDark/emerald/internal/kubernetes"
)

type kubernetesClusterRequest struct {
	Name             string                 `json:"name"`
	SourceType       string                 `json:"source_type"`
	Kubeconfig       string                 `json:"kubeconfig"`
	ContextName      string                 `json:"context_name"`
	DefaultNamespace string                 `json:"default_namespace"`
	Manual           *ik8s.ManualAuthConfig `json:"manual,omitempty"`
}

type kubernetesClusterResponse struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	SourceType       string                 `json:"source_type"`
	Kubeconfig       string                 `json:"kubeconfig,omitempty"`
	HasSecret        bool                   `json:"has_secret"`
	ContextName      string                 `json:"context_name"`
	DefaultNamespace string                 `json:"default_namespace"`
	Server           string                 `json:"server"`
	Manual           *ik8s.ManualAuthConfig `json:"manual,omitempty"`
	CreatedAt        string                 `json:"created_at,omitempty"`
	UpdatedAt        string                 `json:"updated_at,omitempty"`
}

// KubernetesClusterHandler manages Kubernetes cluster settings.
type KubernetesClusterHandler struct {
	store         *query.KubernetesClusterStore
	authService   *auth.Service
	auditLogStore *query.AuditLogStore
}

// NewKubernetesClusterHandler creates a handler for Kubernetes clusters.
func NewKubernetesClusterHandler(store *query.KubernetesClusterStore, opts ...KubernetesClusterHandlerOptions) *KubernetesClusterHandler {
	handler := &KubernetesClusterHandler{store: store}
	if len(opts) > 0 {
		handler.authService = opts[0].AuthService
		handler.auditLogStore = opts[0].AuditLogStore
	}
	return handler
}

type KubernetesClusterHandlerOptions struct {
	AuthService   *auth.Service
	AuditLogStore *query.AuditLogStore
}

// List returns normalized Kubernetes clusters without kubeconfig payloads.
func (h *KubernetesClusterHandler) List(c *fiber.Ctx) error {
	clusters, err := h.store.List(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if clusters == nil {
		c.Type("json")
		return c.SendString("[]")
	}

	responses := make([]kubernetesClusterResponse, 0, len(clusters))
	for _, cluster := range clusters {
		responses = append(responses, kubernetesClusterResponse{
			ID:               cluster.ID,
			Name:             cluster.Name,
			SourceType:       cluster.SourceType,
			ContextName:      cluster.ContextName,
			DefaultNamespace: cluster.DefaultNamespace,
			Server:           cluster.Server,
			CreatedAt:        cluster.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:        cluster.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return c.JSON(responses)
}

// Create validates and stores a Kubernetes cluster.
func (h *KubernetesClusterHandler) Create(c *fiber.Ctx) error {
	var req kubernetesClusterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Name) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	normalized, err := ik8s.NormalizeClusterInput(ik8s.ClusterInput{
		Name:             req.Name,
		SourceType:       req.SourceType,
		Kubeconfig:       req.Kubeconfig,
		ContextName:      req.ContextName,
		DefaultNamespace: req.DefaultNamespace,
		Manual:           req.Manual,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	cluster := &models.KubernetesCluster{
		Name:             strings.TrimSpace(req.Name),
		SourceType:       normalized.SourceType,
		Kubeconfig:       normalized.Kubeconfig,
		ContextName:      normalized.ContextName,
		DefaultNamespace: normalized.DefaultNamespace,
		Server:           normalized.Server,
	}
	if err := h.store.Create(c.Context(), cluster); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	created, err := h.store.GetByID(c.Context(), cluster.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(h.responseFor(created, false))
}

// Get returns a single Kubernetes cluster without secret material.
func (h *KubernetesClusterHandler) Get(c *fiber.Ctx) error {
	cluster, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "kubernetes cluster not found"})
	}

	return c.JSON(h.responseFor(cluster, false))
}

// Update validates and replaces a Kubernetes cluster.
func (h *KubernetesClusterHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	existing, err := h.store.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "kubernetes cluster not found"})
	}

	var req kubernetesClusterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(req.Name) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	kubeconfig := req.Kubeconfig
	manual := req.Manual
	if !bodyIncludesJSONField(c, "kubeconfig") && !bodyIncludesJSONField(c, "manual") {
		kubeconfig = existing.Kubeconfig
		if existing.SourceType == ik8s.SourceTypeManual && existing.Kubeconfig != "" {
			recovered, _, recoverErr := ik8s.RecoverManualConfig(existing.Kubeconfig, existing.ContextName)
			if recoverErr == nil {
				manual = recovered
			}
		}
	}

	normalized, err := ik8s.NormalizeClusterInput(ik8s.ClusterInput{
		Name:             req.Name,
		SourceType:       req.SourceType,
		Kubeconfig:       kubeconfig,
		ContextName:      req.ContextName,
		DefaultNamespace: req.DefaultNamespace,
		Manual:           manual,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	cluster := &models.KubernetesCluster{
		ID:               id,
		Name:             strings.TrimSpace(req.Name),
		SourceType:       normalized.SourceType,
		Kubeconfig:       normalized.Kubeconfig,
		ContextName:      normalized.ContextName,
		DefaultNamespace: normalized.DefaultNamespace,
		Server:           normalized.Server,
	}
	if err := h.store.Update(c.Context(), cluster); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	refreshed, err := h.store.GetByID(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(h.responseFor(refreshed, false))
}

// Delete removes a Kubernetes cluster definition.
func (h *KubernetesClusterHandler) Delete(c *fiber.Ctx) error {
	if err := h.store.Delete(c.Context(), c.Params("id")); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// Test validates a Kubernetes connection draft without storing it.
func (h *KubernetesClusterHandler) Test(c *fiber.Ctx) error {
	var req kubernetesClusterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	result, err := ik8s.TestConnection(c.Context(), ik8s.ClusterInput{
		Name:             req.Name,
		SourceType:       req.SourceType,
		Kubeconfig:       req.Kubeconfig,
		ContextName:      req.ContextName,
		DefaultNamespace: req.DefaultNamespace,
		Manual:           req.Manual,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

func (h *KubernetesClusterHandler) Reveal(c *fiber.Ctx) error {
	session, err := requireRevealAuthorization(c, h.authService)
	if err != nil {
		return c.Status(statusCodeForRevealError(err)).JSON(fiber.Map{"error": err.Error()})
	}

	cluster, err := h.store.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "kubernetes cluster not found"})
	}

	if err := writeAuditLog(c, h.auditLogStore, session, "kubernetes_cluster.reveal", "kubernetes_cluster", cluster.ID, map[string]any{"name": cluster.Name}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to write audit log"})
	}

	return c.JSON(h.responseFor(cluster, true))
}

func (h *KubernetesClusterHandler) responseFor(cluster *models.KubernetesCluster, includeSecrets bool) kubernetesClusterResponse {
	response := kubernetesClusterResponse{
		ID:               cluster.ID,
		Name:             cluster.Name,
		SourceType:       cluster.SourceType,
		HasSecret:        cluster.HasSecret || strings.TrimSpace(cluster.Kubeconfig) != "",
		ContextName:      cluster.ContextName,
		DefaultNamespace: cluster.DefaultNamespace,
		Server:           cluster.Server,
	}
	if !cluster.CreatedAt.IsZero() {
		response.CreatedAt = cluster.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if !cluster.UpdatedAt.IsZero() {
		response.UpdatedAt = cluster.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if includeSecrets {
		response.Kubeconfig = cluster.Kubeconfig
		if cluster.SourceType == ik8s.SourceTypeManual {
			manual, _, err := ik8s.RecoverManualConfig(cluster.Kubeconfig, cluster.ContextName)
			if err == nil {
				response.Manual = manual
			}
		}
	}

	return response
}
