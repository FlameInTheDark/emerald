package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/emerald/internal/nodedefs"
)

type NodeDefinitionsHandler struct {
	service *nodedefs.Service
}

type nodeDefinitionsResponse struct {
	Definitions []nodedefs.Definition `json:"definitions"`
	Plugins     any                   `json:"plugins,omitempty"`
	Error       string                `json:"error,omitempty"`
}

func NewNodeDefinitionsHandler(service *nodedefs.Service) *NodeDefinitionsHandler {
	return &NodeDefinitionsHandler{service: service}
}

func (h *NodeDefinitionsHandler) List(c *fiber.Ctx) error {
	if h == nil || h.service == nil {
		return c.JSON(nodeDefinitionsResponse{Definitions: []nodedefs.Definition{}})
	}

	return c.JSON(h.response(""))
}

func (h *NodeDefinitionsHandler) Refresh(c *fiber.Ctx) error {
	if h == nil || h.service == nil {
		return c.JSON(nodeDefinitionsResponse{Definitions: []nodedefs.Definition{}})
	}

	refreshErr := h.service.RefreshPlugins(c.UserContext())
	if refreshErr != nil {
		return c.JSON(h.response(refreshErr.Error()))
	}

	return c.JSON(h.response(""))
}

func (h *NodeDefinitionsHandler) response(err string) nodeDefinitionsResponse {
	return nodeDefinitionsResponse{
		Definitions: h.service.List(),
		Plugins:     h.service.PluginStatuses(),
		Error:       err,
	}
}
