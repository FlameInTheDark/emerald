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
}

func NewNodeDefinitionsHandler(service *nodedefs.Service) *NodeDefinitionsHandler {
	return &NodeDefinitionsHandler{service: service}
}

func (h *NodeDefinitionsHandler) List(c *fiber.Ctx) error {
	if h == nil || h.service == nil {
		return c.JSON(nodeDefinitionsResponse{Definitions: []nodedefs.Definition{}})
	}

	return c.JSON(nodeDefinitionsResponse{
		Definitions: h.service.List(),
		Plugins:     h.service.PluginStatuses(),
	})
}
