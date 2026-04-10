package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/templating"
	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
)

type ActionExecutor struct {
	Manager  *Manager
	NodeType string
	Outputs  []pluginapi.OutputHandle
}

func (e *ActionExecutor) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e == nil || e.Manager == nil {
		return nil, fmt.Errorf("plugin manager is not configured")
	}

	renderedConfig, err := templating.RenderJSON(config, input)
	if err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	output, err := e.Manager.ExecuteAction(ctx, e.NodeType, renderedConfig, input)
	if err != nil {
		return nil, err
	}

	if err := validateActionOutput(output, e.Outputs); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("encode plugin action output: %w", err)
	}

	return &node.NodeResult{Output: payload}, nil
}

func (e *ActionExecutor) Validate(config json.RawMessage) error {
	if e == nil || e.Manager == nil {
		return fmt.Errorf("plugin manager is not configured")
	}
	return e.Manager.ValidateConfig(context.Background(), e.NodeType, config)
}

type ToolExecutor struct {
	Manager  *Manager
	NodeType string
}

func (e *ToolExecutor) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	result, err := e.ExecuteTool(ctx, config, nil, input)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode plugin tool output: %w", err)
	}

	return &node.NodeResult{Output: payload}, nil
}

func (e *ToolExecutor) Validate(config json.RawMessage) error {
	if e == nil || e.Manager == nil {
		return fmt.Errorf("plugin manager is not configured")
	}
	return e.Manager.ValidateConfig(context.Background(), e.NodeType, config)
}

func (e *ToolExecutor) ToolDefinition(ctx context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	if e == nil || e.Manager == nil {
		return nil, fmt.Errorf("plugin manager is not configured")
	}

	renderedConfig, err := templating.RenderJSON(config, nil)
	if err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	definition, err := e.Manager.ToolDefinition(ctx, e.NodeType, pluginapi.ToolNodeMetadata{
		NodeID: meta.NodeID,
		Label:  meta.Label,
	}, renderedConfig)
	if err != nil {
		return nil, err
	}
	if definition == nil {
		return nil, fmt.Errorf("tool definition is required")
	}

	return &llm.ToolDefinition{
		Type: definition.Type,
		Function: llm.ToolSpec{
			Name:        definition.Function.Name,
			Description: definition.Function.Description,
			Parameters:  definition.Function.Parameters,
		},
	}, nil
}

func (e *ToolExecutor) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error) {
	if e == nil || e.Manager == nil {
		return nil, fmt.Errorf("plugin manager is not configured")
	}

	renderedConfig, err := templating.RenderJSON(config, input)
	if err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	return e.Manager.ExecuteTool(ctx, e.NodeType, renderedConfig, args, input)
}

func validateActionOutput(output any, handles []pluginapi.OutputHandle) error {
	if len(handles) == 0 {
		return nil
	}

	payload, ok := output.(map[string]any)
	if !ok {
		return fmt.Errorf("plugin action must return an object when custom outputs are declared")
	}

	rawMatches, ok := payload["matches"]
	if !ok {
		return fmt.Errorf("plugin action must return a matches object for custom outputs")
	}

	matches, ok := rawMatches.(map[string]any)
	if !ok {
		return fmt.Errorf("plugin action matches must be an object")
	}

	for _, handle := range handles {
		handleID := strings.TrimSpace(handle.ID)
		value, exists := matches[handleID]
		if !exists {
			return fmt.Errorf("plugin action matches is missing handle %q", handleID)
		}
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("plugin action matches[%q] must be a boolean", handleID)
		}
	}

	return nil
}
