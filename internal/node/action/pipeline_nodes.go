package action

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
	"github.com/FlameInTheDark/emerald/internal/templating"
)

type PipelineRunner interface {
	Run(ctx context.Context, pipelineID string, input map[string]any) (*pipeline.RunResult, error)
}

type PipelineCatalog interface {
	List(ctx context.Context) ([]models.Pipeline, error)
	GetByID(ctx context.Context, id string) (*models.Pipeline, error)
}

type runPipelineToolArgumentConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type runPipelineConfig struct {
	PipelineID           string                          `json:"pipelineId"`
	Params               string                          `json:"params"`
	ToolName             string                          `json:"toolName"`
	ToolDescription      string                          `json:"toolDescription"`
	AllowModelPipelineID bool                            `json:"allowModelPipelineId"`
	Arguments            []runPipelineToolArgumentConfig `json:"arguments"`
}

type getPipelineConfig struct {
	PipelineID           string `json:"pipelineId"`
	IncludeDefinition    bool   `json:"includeDefinition"`
	ToolName             string `json:"toolName"`
	ToolDescription      string `json:"toolDescription"`
	AllowModelPipelineID bool   `json:"allowModelPipelineId"`
}

type RunPipelineAction struct {
	Runner PipelineRunner
}

func (e *RunPipelineAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.Runner == nil {
		return nil, fmt.Errorf("pipeline runner is not configured")
	}

	cfg, err := parseRunPipelineConfig(config, input)
	if err != nil {
		return nil, err
	}

	params, err := parsePipelineParams(cfg.Params, input)
	if err != nil {
		return nil, err
	}

	result, err := e.Runner.Run(ctx, cfg.PipelineID, params)
	if err != nil {
		return nil, fmt.Errorf("run pipeline %s: %w", cfg.PipelineID, err)
	}

	data, _ := json.Marshal(buildRunPipelineOutput(result))
	return &node.NodeResult{Output: data}, nil
}

func (e *RunPipelineAction) Validate(config json.RawMessage) error {
	var cfg runPipelineConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.PipelineID) == "" {
		return fmt.Errorf("pipelineId is required")
	}
	return nil
}

type GetPipelineAction struct {
	Pipelines PipelineCatalog
}

func (e *GetPipelineAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	output, err := e.loadPipelineOutput(ctx, config, input)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *GetPipelineAction) Validate(config json.RawMessage) error {
	var cfg getPipelineConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.PipelineID) == "" {
		return fmt.Errorf("pipelineId is required")
	}

	return nil
}

func (e *GetPipelineAction) loadPipelineOutput(ctx context.Context, config json.RawMessage, input map[string]any) (map[string]any, error) {
	if e.Pipelines == nil {
		return nil, fmt.Errorf("pipeline store is not configured")
	}

	cfg, err := parseGetPipelineConfig(config, input)
	if err != nil {
		return nil, err
	}

	return loadPipelineOutputByID(ctx, e.Pipelines, cfg.PipelineID, cfg.IncludeDefinition)
}

type PipelineListToolNode struct {
	Pipelines PipelineCatalog
}

type pipelineListToolArgs struct {
	PipelineID        string `json:"pipelineId"`
	PipelineName      string `json:"pipelineName"`
	IncludeDefinition bool   `json:"includeDefinition"`
}

func (e *PipelineListToolNode) Execute(ctx context.Context, _ json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	output, err := e.listPipelinesOutput(ctx, pipelineListToolArgs{})
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *PipelineListToolNode) Validate(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(config, &raw); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	return nil
}

func (e *PipelineListToolNode) ToolDefinition(_ context.Context, meta node.ToolNodeMetadata, _ json.RawMessage) (*llm.ToolDefinition, error) {
	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(meta.Label, "list_pipelines"),
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
						"description": "Optional pipeline name to filter by.",
					},
					"includeDefinition": map[string]any{
						"type":        "boolean",
						"description": "When true, include nodes, edges, and viewport in the response.",
					},
				},
			},
		},
	}, nil
}

func (e *PipelineListToolNode) ExecuteTool(ctx context.Context, _ json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	toolArgs, err := parsePipelineListToolArgs(args)
	if err != nil {
		return nil, err
	}

	return e.listPipelinesOutput(ctx, toolArgs)
}

func (e *PipelineListToolNode) listPipelinesOutput(ctx context.Context, args pipelineListToolArgs) (map[string]any, error) {
	if e.Pipelines == nil {
		return nil, fmt.Errorf("pipeline store is not configured")
	}

	pipelines, err := e.Pipelines.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}

	filtered := pipelineops.FilterPipelines(pipelines, pipelineops.Reference{
		ID:   args.PipelineID,
		Name: args.PipelineName,
	})

	return pipelineops.BuildListOutput(filtered, args.IncludeDefinition)
}

type PipelineGetToolNode struct {
	Pipelines PipelineCatalog
}

func (e *PipelineGetToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	output, err := (&GetPipelineAction{Pipelines: e.Pipelines}).loadPipelineOutput(ctx, config, input)
	if err != nil {
		return nil, err
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *PipelineGetToolNode) Validate(config json.RawMessage) error {
	_, err := parseGetPipelineToolConfig(config)
	return err
}

func (e *PipelineGetToolNode) ToolDefinition(ctx context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseGetPipelineToolConfig(config)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cfg.ToolDescription)
	if description == "" {
		description = "Get pipeline data for a configured pipeline. By default, the response includes the pipeline definition with nodes, edges, and viewport."
	}
	if e.Pipelines != nil && strings.TrimSpace(cfg.PipelineID) != "" {
		if pipelineModel, err := e.Pipelines.GetByID(ctx, cfg.PipelineID); err == nil && pipelineModel != nil {
			description = fmt.Sprintf("Get pipeline data for %q. By default, the response includes nodes, edges, and viewport data for editing.", pipelineModel.Name)
		}
	}
	if strings.TrimSpace(cfg.ToolDescription) != "" {
		description = strings.TrimSpace(cfg.ToolDescription)
	}

	properties := map[string]any{
		"includeDefinition": map[string]any{
			"type":        "boolean",
			"description": "When true, include nodes, edges, and viewport in the response. Defaults to the node configuration.",
		},
	}
	required := make([]string, 0, 1)
	if cfg.AllowModelPipelineID {
		properties["pipelineId"] = map[string]any{
			"type":        "string",
			"description": "Pipeline ID to load. Use this when you need to choose the target pipeline dynamically.",
		}
		if strings.TrimSpace(cfg.PipelineID) == "" {
			required = append(required, "pipelineId")
		}
	}

	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(strings.TrimSpace(cfg.ToolName), sanitizeToolName(meta.Label, "get_pipeline")),
			Description: description,
			Parameters:  parameters,
		},
	}, nil
}

func (e *PipelineGetToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	if e.Pipelines == nil {
		return nil, fmt.Errorf("pipeline store is not configured")
	}

	cfg, err := parseGetPipelineToolConfig(config)
	if err != nil {
		return nil, err
	}

	toolArgs, err := parseGetPipelineToolArgs(args)
	if err != nil {
		return nil, err
	}

	targetPipelineID := strings.TrimSpace(cfg.PipelineID)
	if cfg.AllowModelPipelineID && strings.TrimSpace(toolArgs.PipelineID) != "" {
		targetPipelineID = strings.TrimSpace(toolArgs.PipelineID)
	}
	if targetPipelineID == "" {
		return nil, fmt.Errorf("pipelineId is required")
	}

	includeDefinition := cfg.IncludeDefinition
	if toolArgs.IncludeDefinition != nil {
		includeDefinition = *toolArgs.IncludeDefinition
	}

	return loadPipelineOutputByID(ctx, e.Pipelines, targetPipelineID, includeDefinition)
}

type PipelineRunToolNode struct {
	Pipelines PipelineCatalog
	Runner    PipelineRunner
}

func (e *PipelineRunToolNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.Runner == nil {
		return nil, fmt.Errorf("pipeline runner is not configured")
	}

	cfg, err := parseRunPipelineToolConfig(config)
	if err != nil {
		return nil, err
	}

	result, err := e.Runner.Run(ctx, cfg.PipelineID, copyParamsMap(input))
	if err != nil {
		return nil, fmt.Errorf("run pipeline %s: %w", cfg.PipelineID, err)
	}

	data, _ := json.Marshal(buildRunPipelineOutput(result))
	return &node.NodeResult{Output: data}, nil
}

func (e *PipelineRunToolNode) Validate(config json.RawMessage) error {
	_, err := parseRunPipelineToolConfig(config)
	return err
}

func (e *PipelineRunToolNode) ToolDefinition(ctx context.Context, meta node.ToolNodeMetadata, config json.RawMessage) (*llm.ToolDefinition, error) {
	cfg, err := parseRunPipelineToolConfig(config)
	if err != nil {
		return nil, err
	}

	description := strings.TrimSpace(cfg.ToolDescription)
	if description == "" {
		description = "Run the configured pipeline manually and optionally pass a params object as input."
	}
	if e.Pipelines != nil && strings.TrimSpace(cfg.PipelineID) != "" {
		if pipelineModel, err := e.Pipelines.GetByID(ctx, cfg.PipelineID); err == nil && pipelineModel != nil {
			description = fmt.Sprintf(
				"Run the pipeline %q manually. Pass params as an object. If that pipeline reaches its Return node, the returned data will be included in the tool result.",
				pipelineModel.Name,
			)
		}
	}
	if cfg.ToolDescription != "" {
		description = strings.TrimSpace(cfg.ToolDescription)
	}

	properties := map[string]any{
		"params": map[string]any{
			"type":        "object",
			"description": "Optional input object passed to the target pipeline as manual execution parameters.",
		},
	}
	required := make([]string, 0, 1)
	if cfg.AllowModelPipelineID {
		properties["pipelineId"] = map[string]any{
			"type":        "string",
			"description": "Pipeline ID to run. Use this when you need to choose the target pipeline dynamically.",
		}
		if strings.TrimSpace(cfg.PipelineID) == "" {
			required = append(required, "pipelineId")
		}
	}
	for _, argument := range cfg.Arguments {
		description := strings.TrimSpace(argument.Description)
		if description == "" {
			description = fmt.Sprintf("Value passed into the called pipeline as arguments.%s.", argument.Name)
		}
		properties[argument.Name] = buildRunPipelineArgumentSchema(description)
		if argument.Required {
			required = append(required, argument.Name)
		}
	}

	parameters := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		parameters["required"] = required
	}

	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        sanitizeToolName(strings.TrimSpace(cfg.ToolName), sanitizeToolName(meta.Label, "run_pipeline")),
			Description: description,
			Parameters:  parameters,
		},
	}, nil
}

func (e *PipelineRunToolNode) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	if e.Runner == nil {
		return nil, fmt.Errorf("pipeline runner is not configured")
	}

	cfg, err := parseRunPipelineToolConfig(config)
	if err != nil {
		return nil, err
	}

	toolArgs, err := parseRunPipelineToolArgsWithConfig(args, cfg.Arguments)
	if err != nil {
		return nil, err
	}

	targetPipelineID := strings.TrimSpace(cfg.PipelineID)
	if cfg.AllowModelPipelineID && strings.TrimSpace(toolArgs.PipelineID) != "" {
		targetPipelineID = strings.TrimSpace(toolArgs.PipelineID)
	}
	if targetPipelineID == "" {
		return nil, fmt.Errorf("pipelineId is required")
	}

	result, err := e.Runner.Run(ctx, targetPipelineID, buildRunPipelineToolInput(toolArgs.Params, toolArgs.Arguments))
	if err != nil {
		return nil, fmt.Errorf("run pipeline %s: %w", targetPipelineID, err)
	}

	return buildRunPipelineOutput(result), nil
}

func parseRunPipelineConfig(config json.RawMessage, input map[string]any) (runPipelineConfig, error) {
	var cfg runPipelineConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return runPipelineConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return runPipelineConfig{}, fmt.Errorf("render config: %w", err)
	}
	if strings.TrimSpace(cfg.PipelineID) == "" {
		return runPipelineConfig{}, fmt.Errorf("pipelineId is required")
	}
	return cfg, nil
}

func parseRunPipelineToolConfig(config json.RawMessage) (runPipelineConfig, error) {
	var cfg runPipelineConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return runPipelineConfig{}, fmt.Errorf("parse config: %w", err)
	}
	arguments, err := normalizeRunPipelineToolArguments(cfg.Arguments)
	if err != nil {
		return runPipelineConfig{}, err
	}
	cfg.Arguments = arguments
	if strings.TrimSpace(cfg.PipelineID) == "" && !cfg.AllowModelPipelineID {
		return runPipelineConfig{}, fmt.Errorf("pipelineId is required")
	}
	return cfg, nil
}

func parseGetPipelineConfig(config json.RawMessage, input map[string]any) (getPipelineConfig, error) {
	cfg, err := parseGetPipelineToolConfig(config)
	if err != nil {
		return getPipelineConfig{}, err
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return getPipelineConfig{}, fmt.Errorf("render config: %w", err)
	}
	if strings.TrimSpace(cfg.PipelineID) == "" {
		return getPipelineConfig{}, fmt.Errorf("pipelineId is required")
	}

	return cfg, nil
}

func parseGetPipelineToolConfig(config json.RawMessage) (getPipelineConfig, error) {
	cfg := getPipelineConfig{IncludeDefinition: true}
	if len(config) == 0 {
		if !cfg.AllowModelPipelineID {
			return getPipelineConfig{}, fmt.Errorf("pipelineId is required")
		}
		return cfg, nil
	}

	if err := json.Unmarshal(config, &cfg); err != nil {
		return getPipelineConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if !hasJSONField(config, "includeDefinition") {
		cfg.IncludeDefinition = true
	}
	if strings.TrimSpace(cfg.PipelineID) == "" && !cfg.AllowModelPipelineID {
		return getPipelineConfig{}, fmt.Errorf("pipelineId is required")
	}

	return cfg, nil
}

func parsePipelineParams(raw string, fallback map[string]any) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return copyParamsMap(fallback), nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("parse params JSON: %w", err)
	}

	params, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("params must be a JSON object")
	}

	return params, nil
}

func parseToolParams(args json.RawMessage) (map[string]any, error) {
	if len(args) == 0 {
		return make(map[string]any), nil
	}

	var payload struct {
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, fmt.Errorf("parse tool args: %w", err)
	}
	if payload.Params == nil {
		return make(map[string]any), nil
	}

	return payload.Params, nil
}

type runPipelineToolArgs struct {
	PipelineID string         `json:"pipelineId"`
	Params     map[string]any `json:"params"`
	Arguments  map[string]any `json:"-"`
}

func parseRunPipelineToolArgs(args json.RawMessage) (runPipelineToolArgs, error) {
	return parseRunPipelineToolArgsWithConfig(args, nil)
}

func parsePipelineListToolArgs(args json.RawMessage) (pipelineListToolArgs, error) {
	if len(args) == 0 {
		return pipelineListToolArgs{}, nil
	}

	var payload pipelineListToolArgs
	if err := json.Unmarshal(args, &payload); err != nil {
		return pipelineListToolArgs{}, fmt.Errorf("parse tool args: %w", err)
	}

	return payload, nil
}

type getPipelineToolArgs struct {
	PipelineID        string `json:"pipelineId"`
	IncludeDefinition *bool  `json:"includeDefinition"`
}

func parseGetPipelineToolArgs(args json.RawMessage) (getPipelineToolArgs, error) {
	if len(args) == 0 {
		return getPipelineToolArgs{}, nil
	}

	var payload getPipelineToolArgs
	if err := json.Unmarshal(args, &payload); err != nil {
		return getPipelineToolArgs{}, fmt.Errorf("parse tool args: %w", err)
	}

	return payload, nil
}

func buildRunPipelineOutput(result *pipeline.RunResult) map[string]any {
	if result == nil {
		return map[string]any{
			"status": "completed",
		}
	}

	output := map[string]any{
		"execution_id":  result.ExecutionID,
		"pipeline_id":   result.PipelineID,
		"pipeline_name": result.PipelineName,
		"status":        result.Status,
		"nodes_run":     result.NodesRun,
		"returned":      result.Returned,
	}
	if result.Returned {
		output["return_value"] = result.ReturnValue
	}

	return output
}

func loadPipelineOutputByID(ctx context.Context, catalog PipelineCatalog, pipelineID string, includeDefinition bool) (map[string]any, error) {
	pipelineID = strings.TrimSpace(pipelineID)
	if pipelineID == "" {
		return nil, fmt.Errorf("pipelineId is required")
	}

	pipelineModel, err := catalog.GetByID(ctx, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("load pipeline %s: %w", pipelineID, err)
	}
	if pipelineModel == nil {
		return nil, fmt.Errorf("pipeline %s not found", pipelineID)
	}

	output, err := pipelineops.BuildPipelineOutput(*pipelineModel, includeDefinition)
	if err != nil {
		return nil, err
	}

	return map[string]any{"pipeline": output}, nil
}

func copyParamsMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}

	copied := make(map[string]any, len(input))
	for key, value := range input {
		copied[key] = value
	}

	return copied
}

func buildRunPipelineToolInput(params map[string]any, arguments map[string]any) map[string]any {
	input := copyParamsMap(params)
	if len(arguments) == 0 {
		return input
	}

	mergedArguments := make(map[string]any)
	if existing, ok := input["arguments"].(map[string]any); ok {
		for key, value := range existing {
			mergedArguments[key] = value
		}
	}
	for key, value := range arguments {
		mergedArguments[key] = value
	}
	input["arguments"] = mergedArguments

	return input
}

func parseRunPipelineToolArgsWithConfig(args json.RawMessage, configuredArguments []runPipelineToolArgumentConfig) (runPipelineToolArgs, error) {
	payload, err := parseToolArgMap(args)
	if err != nil {
		return runPipelineToolArgs{}, err
	}

	pipelineID, _, err := parseOptionalStringArg(payload, "pipelineId")
	if err != nil {
		return runPipelineToolArgs{}, err
	}
	params, _, err := parseOptionalObjectArg(payload, "params")
	if err != nil {
		return runPipelineToolArgs{}, err
	}

	arguments := make(map[string]any, len(configuredArguments))
	for _, configuredArgument := range configuredArguments {
		raw, ok := payload[configuredArgument.Name]
		if !ok {
			if configuredArgument.Required {
				return runPipelineToolArgs{}, fmt.Errorf("%s is required", configuredArgument.Name)
			}
			continue
		}
		if strings.TrimSpace(string(raw)) == "null" {
			if configuredArgument.Required {
				return runPipelineToolArgs{}, fmt.Errorf("%s is required", configuredArgument.Name)
			}
			continue
		}

		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return runPipelineToolArgs{}, fmt.Errorf("parse %s: %w", configuredArgument.Name, err)
		}
		arguments[configuredArgument.Name] = value
	}

	return runPipelineToolArgs{
		PipelineID: strings.TrimSpace(pipelineID),
		Params:     params,
		Arguments:  arguments,
	}, nil
}

func normalizeRunPipelineToolArguments(arguments []runPipelineToolArgumentConfig) ([]runPipelineToolArgumentConfig, error) {
	if len(arguments) == 0 {
		return nil, nil
	}

	normalized := make([]runPipelineToolArgumentConfig, 0, len(arguments))
	seen := make(map[string]struct{}, len(arguments))
	for index, argument := range arguments {
		name := strings.TrimSpace(argument.Name)
		if name == "" {
			return nil, fmt.Errorf("arguments[%d].name is required", index)
		}
		if !isValidToolArgumentName(name) {
			return nil, fmt.Errorf("arguments[%d].name must start with a letter or underscore and contain only letters, numbers, and underscores", index)
		}

		lowerName := strings.ToLower(name)
		switch lowerName {
		case "pipelineid", "params", "arguments":
			return nil, fmt.Errorf("arguments[%d].name %q is reserved", index, name)
		}
		if _, exists := seen[lowerName]; exists {
			return nil, fmt.Errorf("duplicate argument name %q", name)
		}
		seen[lowerName] = struct{}{}

		normalized = append(normalized, runPipelineToolArgumentConfig{
			Name:        name,
			Description: strings.TrimSpace(argument.Description),
			Required:    argument.Required,
		})
	}

	return normalized, nil
}

func buildRunPipelineArgumentSchema(description string) map[string]any {
	schema := map[string]any{
		"description": description,
		"oneOf": []map[string]any{
			{"type": "string"},
			{"type": "number"},
			{"type": "boolean"},
			{
				"type":                 "object",
				"additionalProperties": true,
			},
			{
				"type": "array",
				"items": map[string]any{
					"oneOf": []map[string]any{
						{"type": "string"},
						{"type": "number"},
						{"type": "boolean"},
						{
							"type":                 "object",
							"additionalProperties": true,
						},
					},
				},
			},
		},
	}

	return schema
}

func parseOptionalObjectArg(payload map[string]json.RawMessage, key string) (map[string]any, bool, error) {
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

func hasJSONField(raw json.RawMessage, key string) bool {
	if len(raw) == 0 {
		return false
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}

	_, ok := payload[key]
	return ok
}

func isValidToolArgumentName(name string) bool {
	if name == "" {
		return false
	}

	for index, r := range name {
		if index == 0 {
			if !(unicode.IsLetter(r) || r == '_') {
				return false
			}
			continue
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			return false
		}
	}

	return true
}

func sanitizeToolName(label string, fallback string) string {
	base := strings.TrimSpace(label)
	if base == "" {
		base = fallback
	}

	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(base) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastUnderscore = false
		case !lastUnderscore:
			builder.WriteRune('_')
			lastUnderscore = true
		}
	}

	name := strings.Trim(builder.String(), "_")
	if name == "" {
		name = fallback
	}
	if len(name) > 0 && unicode.IsDigit(rune(name[0])) {
		name = "tool_" + name
	}

	return name
}
