package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/skills"
	"github.com/FlameInTheDark/emerald/internal/templating"
)

type llmAgentConfig struct {
	ProviderID   string  `json:"providerId"`
	Prompt       string  `json:"prompt"`
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"max_tokens"`
	EnableSkills bool    `json:"enableSkills"`
}

type LLMAgentNode struct {
	Providers LLMProviderStore
	Skills    skills.Reader
}

func (e *LLMAgentNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg llmAgentConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	renderInput := copyAgentInput(input)
	if cfg.EnableSkills && e.Skills != nil {
		renderInput["skills"] = e.Skills.SummaryText()
	}

	if err := templating.RenderStrings(&cfg, renderInput); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if strings.TrimSpace(cfg.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1024
	}

	providerConfig, providerModel, err := (&LLMPromptNode{Providers: e.Providers}).resolveProvider(ctx, cfg.ProviderID)
	if err != nil {
		return nil, err
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = providerConfig.Model
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	providerConfig.Model = model

	provider, err := llm.NewProvider(providerConfig)
	if err != nil {
		return nil, fmt.Errorf("initialize provider: %w", err)
	}

	toolRegistry, toolNames, err := buildAgentToolRegistry(ctx, input, cfg.EnableSkills, e.Skills)
	if err != nil {
		return nil, err
	}

	systemPrompt := "You are an automation agent. Use available tools when they help accomplish the task, then provide a final answer."
	if cfg.EnableSkills && e.Skills != nil && strings.TrimSpace(e.Skills.SummaryText()) != "" {
		systemPrompt += " Local skills are enabled for this run. Use the get_skill tool when you need the full text of one of the available skills."
	}

	resp, toolCalls, toolResults, _, err := llm.RunToolChat(
		ctx,
		provider,
		model,
		[]llm.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: cfg.Prompt,
			},
		},
		toolRegistry,
		llm.DefaultMaxToolChatRounds,
	)
	if err != nil {
		return nil, fmt.Errorf("run agent with provider %s: %w", providerModel.Name, err)
	}

	output := map[string]any{
		"providerId":    providerModel.ID,
		"providerName":  providerModel.Name,
		"providerType":  providerModel.ProviderType,
		"prompt":        cfg.Prompt,
		"model":         model,
		"temperature":   cfg.Temperature,
		"max_tokens":    cfg.MaxTokens,
		"content":       resp.Content,
		"toolCalls":     toolCalls,
		"toolResults":   toolResults,
		"tools":         toolNames,
		"skillsEnabled": cfg.EnableSkills,
		"usage":         resp.Usage,
		"status":        "completed",
	}
	if cfg.EnableSkills && e.Skills != nil {
		output["skills"] = e.Skills.SummaryText()
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *LLMAgentNode) Validate(config json.RawMessage) error {
	var cfg llmAgentConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}

type connectedAgentTool struct {
	definition llm.ToolDefinition
	execute    func(ctx context.Context, args json.RawMessage) (any, error)
}

type agentToolRegistry struct {
	tools    []llm.ToolDefinition
	handlers map[string]func(ctx context.Context, args json.RawMessage) (any, error)
}

func (r *agentToolRegistry) GetAllTools() []llm.ToolDefinition {
	return append([]llm.ToolDefinition(nil), r.tools...)
}

func (r *agentToolRegistry) Execute(ctx context.Context, name string, arguments json.RawMessage) (any, error) {
	handler, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("unknown connected tool: %s", name)
	}

	return handler(ctx, arguments)
}

func buildAgentToolRegistry(ctx context.Context, input map[string]any, enableSkills bool, skillStore skills.Reader) (*agentToolRegistry, []string, error) {
	runtime := pipeline.RuntimeFromContext(ctx)
	if runtime == nil {
		return nil, nil, fmt.Errorf("execution runtime is not available")
	}

	currentNodeID := pipeline.CurrentNodeIDFromContext(ctx)
	if currentNodeID == "" {
		return nil, nil, fmt.Errorf("current agent node is not available")
	}

	connectedNodes := runtime.ConnectedToolNodes(currentNodeID)
	registry := &agentToolRegistry{
		tools:    make([]llm.ToolDefinition, 0, len(connectedNodes)),
		handlers: make(map[string]func(ctx context.Context, args json.RawMessage) (any, error), len(connectedNodes)),
	}

	usedNames := make(map[string]int)
	toolNames := make([]string, 0, len(connectedNodes))

	if enableSkills && skillStore != nil {
		skillTool := llm.SkillToolDefinition()
		registry.tools = append(registry.tools, skillTool)
		registry.handlers[skillTool.Function.Name] = func(toolCtx context.Context, args json.RawMessage) (any, error) {
			return llm.ExecuteSkillTool(toolCtx, skillStore, args)
		}
		usedNames[skillTool.Function.Name] = 1
		toolNames = append(toolNames, skillTool.Function.Name)
	}

	for _, flowNode := range connectedNodes {
		toolNodeType, toolLabel, toolConfig := decodeToolFlowNode(flowNode)
		executor, err := runtime.Registry.Get(toolNodeType)
		if err != nil {
			return nil, nil, fmt.Errorf("load tool node %s: %w", flowNode.ID, err)
		}

		toolExecutor, ok := executor.(node.ToolNodeExecutor)
		if !ok {
			return nil, nil, fmt.Errorf("connected node %s (%s) is not a tool node", flowNode.ID, toolNodeType)
		}

		definition, err := toolExecutor.ToolDefinition(ctx, node.ToolNodeMetadata{
			NodeID: flowNode.ID,
			Label:  toolLabel,
		}, toolConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("build tool definition for %s: %w", flowNode.ID, err)
		}
		if definition == nil {
			return nil, nil, fmt.Errorf("tool definition for %s is empty", flowNode.ID)
		}

		toolName := makeUniqueToolName(definition.Function.Name, usedNames)
		definition.Function.Name = toolName

		toolCopy := toolExecutor
		configCopy := append(json.RawMessage(nil), toolConfig...)
		registry.tools = append(registry.tools, *definition)
		registry.handlers[toolName] = func(toolCtx context.Context, args json.RawMessage) (any, error) {
			return toolCopy.ExecuteTool(toolCtx, configCopy, args, input)
		}
		toolNames = append(toolNames, toolName)
	}

	return registry, toolNames, nil
}

func copyAgentInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}

	copied := make(map[string]any, len(input))
	for key, value := range input {
		copied[key] = value
	}

	return copied
}

func decodeToolFlowNode(flowNode pipeline.FlowNode) (node.NodeType, string, json.RawMessage) {
	nodeType := node.NodeType(flowNode.Type)
	label := flowNode.ID
	configData := flowNode.Data

	if len(flowNode.Data) == 0 {
		return nodeType, label, configData
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(flowNode.Data, &payload); err != nil {
		return nodeType, label, configData
	}

	if rawType, ok := payload["type"]; ok {
		var typeString string
		if err := json.Unmarshal(rawType, &typeString); err == nil && strings.TrimSpace(typeString) != "" {
			nodeType = node.NodeType(typeString)
		}
	}
	if rawLabel, ok := payload["label"]; ok {
		var labelString string
		if err := json.Unmarshal(rawLabel, &labelString); err == nil && strings.TrimSpace(labelString) != "" {
			label = labelString
		}
	}
	if rawConfig, ok := payload["config"]; ok {
		configData = rawConfig
	}

	return nodeType, label, configData
}

func makeUniqueToolName(name string, used map[string]int) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "tool"
	}

	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}

	return fmt.Sprintf("%s_%d", base, count+1)
}
