package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
)

func (r *ToolRegistry) registerPipelineTools() {
	if r.pipelineStore == nil {
		return
	}

	service := pipelineops.NewService(r.pipelineStore, r.pipelineReloader)

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "list_pipelines",
			Description: "List available pipelines. Optionally filter by pipelineId or pipelineName, and set includeDefinition to true when you need nodes, edges, and viewport data for editing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{
						"type":        "string",
						"description": "Optional pipeline ID to filter by.",
					},
					"pipelineName": map[string]any{
						"type":        "string",
						"description": "Optional exact pipeline name to filter by.",
					},
					"includeDefinition": map[string]any{
						"type":        "boolean",
						"description": "When true, include nodes, edges, and viewport in the response.",
					},
				},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		payload, err := parsePipelineToolArgMap(args)
		if err != nil {
			return nil, err
		}

		pipelineID, _, err := parsePipelineToolOptionalString(payload, "pipelineId")
		if err != nil {
			return nil, err
		}
		pipelineName, _, err := parsePipelineToolOptionalString(payload, "pipelineName")
		if err != nil {
			return nil, err
		}
		includeDefinition, err := parsePipelineToolOptionalBool(payload, "includeDefinition")
		if err != nil {
			return nil, err
		}

		pipelines, err := service.List(ctx, pipelineops.Reference{
			ID:   strings.TrimSpace(pipelineID),
			Name: strings.TrimSpace(pipelineName),
		})
		if err != nil {
			return nil, err
		}

		return pipelineops.BuildListOutput(pipelines, includeDefinition)
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "get_pipeline",
			Description: "Get pipeline data for a single pipeline by pipelineId or exact pipelineName. By default, the response includes nodes, edges, and viewport for editing.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{
						"type":        "string",
						"description": "Pipeline ID to load.",
					},
					"pipelineName": map[string]any{
						"type":        "string",
						"description": "Exact pipeline name to load when pipelineId is unknown.",
					},
					"includeDefinition": map[string]any{
						"type":        "boolean",
						"description": "When true, include nodes, edges, and viewport in the response. Defaults to true.",
					},
				},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		payload, err := parsePipelineToolArgMap(args)
		if err != nil {
			return nil, err
		}

		ref, err := parsePipelineToolReference(payload)
		if err != nil {
			return nil, err
		}

		includeDefinition := true
		if _, ok := payload["includeDefinition"]; ok {
			includeDefinition, err = parsePipelineToolOptionalBool(payload, "includeDefinition")
			if err != nil {
				return nil, err
			}
		}

		pipelineModel, err := service.Resolve(ctx, ref)
		if err != nil {
			return nil, err
		}

		output, err := pipelineops.BuildPipelineOutput(*pipelineModel, includeDefinition)
		if err != nil {
			return nil, err
		}

		return map[string]any{"pipeline": output}, nil
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "create_pipeline",
			Description: "Create a new pipeline. Provide a name and, when needed, nodes, edges, and an optional viewport using the Emerald flow JSON schema.",
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
					"nodes":    pipelineToolArraySchema("Optional pipeline nodes using the app flow JSON format."),
					"edges":    pipelineToolArraySchema("Optional pipeline edges using the app flow JSON format."),
					"viewport": pipelineToolObjectSchema("Optional React Flow viewport object."),
				},
				"required": []string{"name"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		payload, err := parsePipelineToolArgMap(args)
		if err != nil {
			return nil, err
		}

		name, present, err := parsePipelineToolOptionalString(payload, "name")
		if err != nil {
			return nil, err
		}
		if !present || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("name is required")
		}

		description, _, err := parsePipelineToolOptionalDescription(payload, "description")
		if err != nil {
			return nil, err
		}
		status, _, err := parsePipelineToolOptionalString(payload, "status")
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
		if err := service.Create(ctx, pipelineModel); err != nil {
			return nil, err
		}

		created, err := service.Resolve(ctx, pipelineops.Reference{ID: pipelineModel.ID})
		if err != nil {
			return nil, fmt.Errorf("load created pipeline %s: %w", pipelineModel.ID, err)
		}

		return buildPipelineToolMutationOutput("created", created, true)
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "update_pipeline",
			Description: "Update an existing pipeline. Identify it by pipelineId or exact pipelineName. Only provided fields are changed.",
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
					"nodes":    pipelineToolArraySchema("Optional full replacement node array."),
					"edges":    pipelineToolArraySchema("Optional full replacement edge array."),
					"viewport": pipelineToolObjectSchema("Optional replacement viewport object. Use null to clear it."),
				},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		payload, err := parsePipelineToolArgMap(args)
		if err != nil {
			return nil, err
		}

		ref, err := parsePipelineToolReference(payload)
		if err != nil {
			return nil, err
		}

		existing, err := service.Resolve(ctx, ref)
		if err != nil {
			return nil, err
		}

		updated := *existing

		if name, present, err := parsePipelineToolOptionalString(payload, "name"); err != nil {
			return nil, err
		} else if present {
			if strings.TrimSpace(name) == "" {
				return nil, fmt.Errorf("name cannot be empty")
			}
			updated.Name = strings.TrimSpace(name)
		}

		if description, present, err := parsePipelineToolOptionalDescription(payload, "description"); err != nil {
			return nil, err
		} else if present {
			updated.Description = description
		}

		if status, present, err := parsePipelineToolOptionalString(payload, "status"); err != nil {
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

		if err := service.Update(ctx, &updated); err != nil {
			return nil, err
		}

		current, err := service.Resolve(ctx, pipelineops.Reference{ID: updated.ID})
		if err != nil {
			return nil, fmt.Errorf("load updated pipeline %s: %w", updated.ID, err)
		}

		return buildPipelineToolMutationOutput("updated", current, true)
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "delete_pipeline",
			Description: "Delete an existing pipeline by pipelineId or exact pipelineName.",
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
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		payload, err := parsePipelineToolArgMap(args)
		if err != nil {
			return nil, err
		}

		ref, err := parsePipelineToolReference(payload)
		if err != nil {
			return nil, err
		}

		deleted, err := service.Delete(ctx, ref)
		if err != nil {
			return nil, err
		}

		return buildPipelineToolMutationOutput("deleted", deleted, false)
	})

	if r.pipelineRunner != nil {
		r.Register(ToolDefinition{
			Type: "function",
			Function: ToolSpec{
				Name:        "run_pipeline",
				Description: "Run an existing pipeline manually. Identify it by pipelineId or exact pipelineName and optionally pass a params object as the manual execution input.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pipelineId": map[string]any{
							"type":        "string",
							"description": "Pipeline ID to run.",
						},
						"pipelineName": map[string]any{
							"type":        "string",
							"description": "Exact pipeline name to run when pipelineId is unknown.",
						},
						"params": pipelineToolObjectSchema("Optional input object passed to the pipeline as manual execution parameters."),
					},
				},
			},
		}, func(ctx context.Context, args json.RawMessage) (any, error) {
			payload, err := parsePipelineToolArgMap(args)
			if err != nil {
				return nil, err
			}

			ref, err := parsePipelineToolReference(payload)
			if err != nil {
				return nil, err
			}

			pipelineModel, err := service.Resolve(ctx, ref)
			if err != nil {
				return nil, err
			}

			params, _, err := parsePipelineToolOptionalObject(payload, "params")
			if err != nil {
				return nil, err
			}

			result, err := r.pipelineRunner.Run(ctx, pipelineModel.ID, params)
			if err != nil {
				return nil, fmt.Errorf("run pipeline %s: %w", pipelineModel.Name, err)
			}

			return buildPipelineToolRunOutput(result), nil
		})
	}

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "activate_pipeline",
			Description: "Activate an existing pipeline so its cron and trigger nodes can run automatically.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{
						"type":        "string",
						"description": "Pipeline ID to activate.",
					},
					"pipelineName": map[string]any{
						"type":        "string",
						"description": "Exact pipeline name to activate when pipelineId is unknown.",
					},
				},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		return updatePipelineStatusTool(ctx, service, args, pipelineops.StatusActive, "activated")
	})

	r.Register(ToolDefinition{
		Type: "function",
		Function: ToolSpec{
			Name:        "deactivate_pipeline",
			Description: "Deactivate an existing pipeline so its cron and trigger nodes stop running automatically. Manual runs still remain available.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pipelineId": map[string]any{
						"type":        "string",
						"description": "Pipeline ID to deactivate.",
					},
					"pipelineName": map[string]any{
						"type":        "string",
						"description": "Exact pipeline name to deactivate when pipelineId is unknown.",
					},
				},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (any, error) {
		return updatePipelineStatusTool(ctx, service, args, pipelineops.StatusDraft, "deactivated")
	})
}

func parsePipelineToolArgMap(args json.RawMessage) (map[string]json.RawMessage, error) {
	if len(args) == 0 {
		return make(map[string]json.RawMessage), nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, fmt.Errorf("parse tool args: %w", err)
	}

	return payload, nil
}

func parsePipelineToolOptionalString(payload map[string]json.RawMessage, key string) (string, bool, error) {
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

func parsePipelineToolOptionalDescription(payload map[string]json.RawMessage, key string) (*string, bool, error) {
	value, present, err := parsePipelineToolOptionalString(payload, key)
	if err != nil || !present {
		return nil, present, err
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true, nil
	}

	return &value, true, nil
}

func parsePipelineToolOptionalBool(payload map[string]json.RawMessage, key string) (bool, error) {
	raw, ok := payload[key]
	if !ok {
		return false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return false, nil
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}

	return value, nil
}

func parsePipelineToolOptionalObject(payload map[string]json.RawMessage, key string) (map[string]any, bool, error) {
	raw, ok := payload[key]
	if !ok {
		return map[string]any{}, false, nil
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return map[string]any{}, true, nil
	}

	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, true, fmt.Errorf("parse %s: %w", key, err)
	}
	if value == nil {
		value = map[string]any{}
	}

	return value, true, nil
}

func parsePipelineToolReference(payload map[string]json.RawMessage) (pipelineops.Reference, error) {
	pipelineID, _, err := parsePipelineToolOptionalString(payload, "pipelineId")
	if err != nil {
		return pipelineops.Reference{}, err
	}
	pipelineName, _, err := parsePipelineToolOptionalString(payload, "pipelineName")
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

func buildPipelineToolMutationOutput(status string, pipelineModel *models.Pipeline, includeDefinition bool) (map[string]any, error) {
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

func buildPipelineToolRunOutput(result *ToolPipelineRunResult) map[string]any {
	if result == nil {
		return map[string]any{"status": "completed"}
	}

	output := map[string]any{
		"status":        result.Status,
		"execution_id":  result.ExecutionID,
		"pipeline_id":   result.PipelineID,
		"pipeline_name": result.PipelineName,
		"nodes_run":     result.NodesRun,
	}
	if result.Returned {
		output["returned"] = true
		output["return_value"] = result.ReturnValue
	}

	return output
}

func updatePipelineStatusTool(
	ctx context.Context,
	service *pipelineops.Service,
	args json.RawMessage,
	status string,
	mutation string,
) (map[string]any, error) {
	payload, err := parsePipelineToolArgMap(args)
	if err != nil {
		return nil, err
	}

	ref, err := parsePipelineToolReference(payload)
	if err != nil {
		return nil, err
	}

	current, err := service.Resolve(ctx, ref)
	if err != nil {
		return nil, err
	}

	updated := *current
	updated.Status = status
	if err := service.Update(ctx, &updated); err != nil {
		return nil, err
	}

	current, err = service.Resolve(ctx, pipelineops.Reference{ID: updated.ID})
	if err != nil {
		return nil, fmt.Errorf("load %s pipeline %s: %w", mutation, updated.ID, err)
	}

	return buildPipelineToolMutationOutput(mutation, current, false)
}

func pipelineToolArraySchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
}

func pipelineToolObjectSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
	}
}
