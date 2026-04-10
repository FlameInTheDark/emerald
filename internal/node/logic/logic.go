package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/templating"
	"github.com/expr-lang/expr"
)

type ConditionNode struct{}

type conditionConfig struct {
	Expression string `json:"expression"`
}

func (e *ConditionNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg conditionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	result, evalErr := evaluateExpression(cfg.Expression, input)

	output := map[string]any{
		"condition": cfg.Expression,
		"input":     input,
		"result":    result,
	}
	if evalErr != nil {
		output["error"] = evalErr.Error()
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ConditionNode) Validate(config json.RawMessage) error {
	var cfg conditionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Expression == "" {
		return fmt.Errorf("expression is required")
	}
	return nil
}

func evaluateExpression(expression string, input map[string]any) (bool, error) {
	env := make(map[string]any, len(input)+1)
	for key, value := range input {
		env[key] = value
	}
	env["input"] = input

	output, err := expr.Eval(expression, env)
	if err != nil {
		return false, fmt.Errorf("eval: %w", err)
	}

	result, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("expression must evaluate to a boolean, got %T", output)
	}

	return result, nil
}

type SwitchNode struct{}

type switchCondition struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Expression string `json:"expression"`
}

type switchConfig struct {
	Conditions []switchCondition `json:"conditions"`
	Key        string            `json:"key"`
	Cases      []string          `json:"cases"`
	Default    string            `json:"default"`
}

func (e *SwitchNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg switchConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if len(cfg.Conditions) > 0 {
		return e.executeConditions(cfg, input)
	}

	value, ok := input[cfg.Key]
	if !ok {
		value = cfg.Default
	}

	output := map[string]any{
		"key":   cfg.Key,
		"value": value,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *SwitchNode) Validate(config json.RawMessage) error {
	var cfg switchConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if len(cfg.Conditions) > 0 {
		for i, condition := range cfg.Conditions {
			if strings.TrimSpace(condition.Expression) == "" {
				return fmt.Errorf("condition %d expression is required", i+1)
			}
		}
		return nil
	}
	if cfg.Key == "" {
		return fmt.Errorf("at least one condition is required")
	}
	return nil
}

func (e *SwitchNode) executeConditions(cfg switchConfig, input map[string]any) (*node.NodeResult, error) {
	conditions := make([]map[string]any, 0, len(cfg.Conditions))
	matches := make(map[string]bool, len(cfg.Conditions)+1)
	matched := make([]map[string]any, 0, len(cfg.Conditions))
	hasMatch := false

	for i, condition := range cfg.Conditions {
		conditionID := normalizeSwitchConditionID(condition.ID, i)
		conditionLabel := normalizeSwitchConditionLabel(condition.Label, i)
		result, evalErr := evaluateExpression(condition.Expression, input)

		conditionOutput := map[string]any{
			"id":         conditionID,
			"label":      conditionLabel,
			"expression": condition.Expression,
			"result":     result,
		}
		if evalErr != nil {
			conditionOutput["error"] = evalErr.Error()
		}

		matches[conditionID] = result
		conditions = append(conditions, conditionOutput)

		if result {
			hasMatch = true
			matched = append(matched, map[string]any{
				"id":         conditionID,
				"label":      conditionLabel,
				"expression": condition.Expression,
			})
		}
	}

	matches["default"] = !hasMatch

	output := map[string]any{
		"conditions":     conditions,
		"matches":        matches,
		"matched":        matched,
		"matchedCount":   len(matched),
		"hasMatch":       hasMatch,
		"defaultMatched": !hasMatch,
		"input":          input,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func normalizeSwitchConditionID(id string, index int) string {
	if trimmed := strings.TrimSpace(id); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("condition-%d", index+1)
}

func normalizeSwitchConditionLabel(label string, index int) string {
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("Condition %d", index+1)
}

type llmPromptConfig struct {
	ProviderID  string  `json:"providerId"`
	Prompt      string  `json:"prompt"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

type LLMProviderStore interface {
	GetByID(ctx context.Context, id string) (*models.LLMProvider, error)
	GetDefault(ctx context.Context) (*models.LLMProvider, error)
}

type LLMPromptNode struct {
	Providers LLMProviderStore
}

func (e *LLMPromptNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg llmPromptConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 1024
	}

	providerConfig, providerModel, err := e.resolveProvider(ctx, cfg.ProviderID)
	if err != nil {
		return nil, err
	}

	model := cfg.Model
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

	resp, err := provider.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "user", Content: cfg.Prompt},
		},
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("run prompt with provider %s: %w", providerModel.Name, err)
	}

	output := map[string]any{
		"providerId":   providerModel.ID,
		"providerName": providerModel.Name,
		"providerType": providerModel.ProviderType,
		"prompt":       cfg.Prompt,
		"model":        model,
		"temperature":  cfg.Temperature,
		"max_tokens":   cfg.MaxTokens,
		"content":      resp.Content,
		"toolCalls":    resp.ToolCalls,
		"usage":        resp.Usage,
		"status":       "completed",
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *LLMPromptNode) Validate(config json.RawMessage) error {
	var cfg llmPromptConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}

func (e *LLMPromptNode) resolveProvider(ctx context.Context, providerID string) (llm.Config, *models.LLMProvider, error) {
	if e.Providers == nil {
		return llm.Config{}, nil, fmt.Errorf("LLM provider store is not configured")
	}

	var providerModel *models.LLMProvider
	var err error
	if providerID != "" {
		providerModel, err = e.Providers.GetByID(ctx, providerID)
		if err != nil {
			return llm.Config{}, nil, fmt.Errorf("load provider %s: %w", providerID, err)
		}
	} else {
		providerModel, err = e.Providers.GetDefault(ctx)
		if err != nil {
			return llm.Config{}, nil, fmt.Errorf("load default provider: %w", err)
		}
	}

	config, err := llm.ConfigFromModel(providerModel)
	if err != nil {
		return llm.Config{}, nil, fmt.Errorf("build provider config: %w", err)
	}

	return config, providerModel, nil
}
