package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
)

type PipelineMutationManager interface {
	Create(ctx context.Context, pipeline *models.Pipeline) error
	Update(ctx context.Context, pipeline *models.Pipeline) error
	Delete(ctx context.Context, ref pipelineops.Reference) (*models.Pipeline, error)
	Resolve(ctx context.Context, ref pipelineops.Reference) (*models.Pipeline, error)
}

type pipelineMutationToolConfig struct {
	ToolName        string `json:"toolName"`
	ToolDescription string `json:"toolDescription"`
}

type PipelineCreateToolNode struct {
	Manager PipelineMutationManager
}

type PipelineUpdateToolNode struct {
	Manager PipelineMutationManager
}

type PipelineDeleteToolNode struct {
	Manager PipelineMutationManager
}

func (e *PipelineCreateToolNode) Execute(ctx context.Context, config json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	output, err := e.ExecuteTool(ctx, config, nil, nil)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *PipelineCreateToolNode) Validate(config json.RawMessage) error {
	_, err := parsePipelineMutationToolConfig(config)
	return err
}

func (e *PipelineCreateToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parsePipelineMutationToolConfig(config)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cfg.ToolDescription)
	if description == "" {
		description = "Create a new pipeline. Provide a name and, when needed, nodes, edges, and an optional viewport using the Emerald flow JSON schema."
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizePipelineMutationToolName(cfg.ToolName, meta.Label, "create_pipeline"),
			Description: description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Pipeline name.",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional pipeline description.",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "Optional pipeline status: draft, active, or archived. Defaults to draft.",
					},
					"nodes":    genericToolArraySchema("Optional pipeline nodes using the app flow JSON format."),
					"edges":    genericToolArraySchema("Optional pipeline edges using the app flow JSON format."),
					"viewport": genericToolObjectSchema("Optional React Flow viewport object."),
				},
				"required": []string{"name"},
			},
		},
	}, nil
}

func (e *PipelineCreateToolNode) ExecuteTool(ctx context.Context, _ json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	if e.Manager == nil {
		return nil, fmt.Errorf("pipeline manager is not configured")
	}

	payload, err := parseToolArgMap(args)
	if err != nil {
		return nil, err
	}

	name, present, err := parseOptionalStringArg(payload, "name")
	if err != nil {
		return nil, err
	}
	if !present || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}

	description, _, err := parseOptionalDescriptionArg(payload, "description")
	if err != nil {
		return nil, err
	}

	status, _, err := parseOptionalStringArg(payload, "status")
	if err != nil {
		return nil, err
	}

	nodesJSON, err := pipelineops.CanonicalizeJSON(payload["nodes"], "[]", "nodes")
	if err != nil {
		return nil, err
	}
	edgesJSON, err := pipelineops.CanonicalizeJSON(payload["edges"], "[]", "edges")
	if err != nil {
		return nil, err
	}
	viewportJSON, err := pipelineops.CanonicalizeJSONPointer(payload["viewport"], "viewport")
	if err != nil {
		return nil, err
	}

	pipelineModel := &models.Pipeline{
		Name:        strings.TrimSpace(name),
		Description: description,
		Status:      status,
		Nodes:       nodesJSON,
		Edges:       edgesJSON,
		Viewport:    viewportJSON,
	}
	if err := e.Manager.Create(ctx, pipelineModel); err != nil {
		return nil, err
	}

	created, err := e.Manager.Resolve(ctx, pipelineops.Reference{ID: pipelineModel.ID})
	if err != nil {
		return nil, fmt.Errorf("load created pipeline %s: %w", pipelineModel.ID, err)
	}

	return buildPipelineMutationOutput("created", created, true)
}

func (e *PipelineUpdateToolNode) Execute(ctx context.Context, config json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	output, err := e.ExecuteTool(ctx, config, nil, nil)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *PipelineUpdateToolNode) Validate(config json.RawMessage) error {
	_, err := parsePipelineMutationToolConfig(config)
	return err
}

func (e *PipelineUpdateToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parsePipelineMutationToolConfig(config)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cfg.ToolDescription)
	if description == "" {
		description = "Update an existing pipeline. Identify it by pipelineId or exact pipelineName. Only provided fields are changed."
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizePipelineMutationToolName(cfg.ToolName, meta.Label, "update_pipeline"),
			Description: description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{
						"type":        "string",
						"description": "Pipeline ID to update.",
					},
					"pipelineName": map[string]any{
						"type":        "string",
						"description": "Exact pipeline name to update when pipelineId is unknown.",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Optional new pipeline name.",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional new pipeline description. Use an empty string to clear it.",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "Optional new pipeline status: draft, active, or archived.",
					},
					"nodes":    genericToolArraySchema("Optional full replacement node array."),
					"edges":    genericToolArraySchema("Optional full replacement edge array."),
					"viewport": genericToolObjectSchema("Optional replacement viewport object. Use null to clear it."),
				},
			},
		},
	}, nil
}

func (e *PipelineUpdateToolNode) ExecuteTool(ctx context.Context, _ json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	if e.Manager == nil {
		return nil, fmt.Errorf("pipeline manager is not configured")
	}

	payload, err := parseToolArgMap(args)
	if err != nil {
		return nil, err
	}

	ref, err := parsePipelineReference(payload)
	if err != nil {
		return nil, err
	}

	existing, err := e.Manager.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}

	updated := *existing

	if name, present, err := parseOptionalStringArg(payload, "name"); err != nil {
		return nil, err
	} else if present {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		updated.Name = strings.TrimSpace(name)
	}

	if description, present, err := parseOptionalDescriptionArg(payload, "description"); err != nil {
		return nil, err
	} else if present {
		updated.Description = description
	}

	if status, present, err := parseOptionalStringArg(payload, "status"); err != nil {
		return nil, err
	} else if present {
		if strings.TrimSpace(status) == "" {
			return nil, fmt.Errorf("status cannot be empty")
		}
		updated.Status = status
	}

	if rawNodes, ok := payload["nodes"]; ok {
		nodesJSON, err := pipelineops.CanonicalizeJSON(rawNodes, "[]", "nodes")
		if err != nil {
			return nil, err
		}
		updated.Nodes = nodesJSON
	}
	if rawEdges, ok := payload["edges"]; ok {
		edgesJSON, err := pipelineops.CanonicalizeJSON(rawEdges, "[]", "edges")
		if err != nil {
			return nil, err
		}
		updated.Edges = edgesJSON
	}
	if rawViewport, ok := payload["viewport"]; ok {
		viewportJSON, err := pipelineops.CanonicalizeJSONPointer(rawViewport, "viewport")
		if err != nil {
			return nil, err
		}
		updated.Viewport = viewportJSON
	}

	if err := e.Manager.Update(ctx, &updated); err != nil {
		return nil, err
	}

	current, err := e.Manager.Resolve(ctx, pipelineops.Reference{ID: updated.ID})
	if err != nil {
		return nil, fmt.Errorf("load updated pipeline %s: %w", updated.ID, err)
	}

	return buildPipelineMutationOutput("updated", current, true)
}

func (e *PipelineDeleteToolNode) Execute(ctx context.Context, config json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	output, err := e.ExecuteTool(ctx, config, nil, nil)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *PipelineDeleteToolNode) Validate(config json.RawMessage) error {
	_, err := parsePipelineMutationToolConfig(config)
	return err
}

func (e *PipelineDeleteToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parsePipelineMutationToolConfig(config)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cfg.ToolDescription)
	if description == "" {
		description = "Delete an existing pipeline by pipelineId or exact pipelineName."
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizePipelineMutationToolName(cfg.ToolName, meta.Label, "delete_pipeline"),
			Description: description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{
						"type":        "string",
						"description": "Pipeline ID to delete.",
					},
					"pipelineName": map[string]any{
						"type":        "string",
						"description": "Exact pipeline name to delete when pipelineId is unknown.",
					},
				},
			},
		},
	}, nil
}

func (e *PipelineDeleteToolNode) ExecuteTool(ctx context.Context, _ json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	if e.Manager == nil {
		return nil, fmt.Errorf("pipeline manager is not configured")
	}

	payload, err := parseToolArgMap(args)
	if err != nil {
		return nil, err
	}

	ref, err := parsePipelineReference(payload)
	if err != nil {
		return nil, err
	}

	deleted, err := e.Manager.Delete(ctx, ref)
	if err != nil {
		return nil, err
	}

	return buildPipelineMutationOutput("deleted", deleted, false)
}

func parsePipelineMutationToolConfig(config json.RawMessage) (pipelineMutationToolConfig, error) {
	if len(config) == 0 {
		return pipelineMutationToolConfig{}, nil
	}

	var cfg pipelineMutationToolConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return pipelineMutationToolConfig{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func parseToolArgMap(args json.RawMessage) (map[string]json.RawMessage, error) {
	if len(args) == 0 {
		return make(map[string]json.RawMessage), nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, fmt.Errorf("parse tool args: %w", err)
	}

	return payload, nil
}

func parseOptionalStringArg(payload map[string]json.RawMessage, key string) (string, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return "", false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return "", true, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, true, nil
}

func parseOptionalDescriptionArg(payload map[string]json.RawMessage, key string) (*string, bool, error) {
	value, present, err := parseOptionalStringArg(payload, key)
	if err != nil || !present {
		return nil, present, err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true, nil
	}

	return &value, true, nil
}

func parsePipelineReference(payload map[string]json.RawMessage) (pipelineops.Reference, error) {
	pipelineID, _, err := parseOptionalStringArg(payload, "pipelineId")
	if err != nil {
		return pipelineops.Reference{}, err
	}
	pipelineName, _, err := parseOptionalStringArg(payload, "pipelineName")
	if err != nil {
		return pipelineops.Reference{}, err
	}

	ref := pipelineops.Reference{
		ID:   strings.TrimSpace(pipelineID),
		Name: strings.TrimSpace(pipelineName),
	}
	if ref.ID == "" && ref.Name == "" {
		return pipelineops.Reference{}, fmt.Errorf("pipelineId or pipelineName is required")
	}

	return ref, nil
}

func buildPipelineMutationOutput(status string, pipelineModel *models.Pipeline, includeDefinition bool) (map[string]any, error) {
	if pipelineModel == nil {
		return map[string]any{"status": status}, nil
	}

	output, err := pipelineops.BuildPipelineOutput(*pipelineModel, includeDefinition)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"status":   status,
		"pipeline": output,
	}, nil
}

func sanitizePipelineMutationToolName(customName string, label string, fallback string) string {
	if strings.TrimSpace(customName) != "" {
		return sanitizeToolName(customName, fallback)
	}

	return sanitizeToolName(label, fallback)
}

func genericToolArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
}

func genericToolObjectSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
	}
}
