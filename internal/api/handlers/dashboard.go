package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/scheduler"
)

type DashboardHandler struct {
	clusters   *query.ClusterStore
	pipelines  *query.PipelineStore
	executions *query.ExecutionStore
	channels   *query.ChannelStore
	scheduler  *scheduler.Scheduler
}

func NewDashboardHandler(
	clusters *query.ClusterStore,
	pipelines *query.PipelineStore,
	executions *query.ExecutionStore,
	channels *query.ChannelStore,
	scheduler *scheduler.Scheduler,
) *DashboardHandler {
	return &DashboardHandler{
		clusters:   clusters,
		pipelines:  pipelines,
		executions: executions,
		channels:   channels,
		scheduler:  scheduler,
	}
}

func (h *DashboardHandler) Stats(c *fiber.Ctx) error {
	ctx := c.Context()

	clusterList, err := h.clusters.List(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	pipelinesCount, err := h.pipelines.Count(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	activePipelines, err := h.pipelines.CountByStatus(ctx, "active")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	executions24h, err := h.executions.CountSince(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	channelsCount, err := h.channels.Count(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	stats := models.DashboardStats{
		Clusters:        len(clusterList),
		Pipelines:       pipelinesCount,
		ActivePipelines: activePipelines,
		Executions24h:   executions24h,
		Channels:        channelsCount,
	}
	if h.scheduler != nil {
		stats.ActiveJobs = h.scheduler.ActiveJobCount()
	}

	return c.JSON(stats)
}
