package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/templating"
)

type returnConfig struct {
	Value string `json:"value"`
}

type ReturnNode struct{}

func (e *ReturnNode) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg returnConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStrings(&cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	returnValue, err := parseReturnValue(cfg.Value, input)
	if err != nil {
		return nil, err
	}

	output := map[string]any{
		"status": "returned",
		"value":  returnValue,
	}
	data, _ := json.Marshal(output)

	return &node.NodeResult{
		Output:      data,
		ReturnValue: returnValue,
	}, nil
}

func (e *ReturnNode) Validate(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}

	var cfg returnConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	return nil
}

func parseReturnValue(raw string, input map[string]any) (any, error) {
	if strings.TrimSpace(raw) == "" {
		return copyReturnMap(input), nil
	}

	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value, nil
	}

	return raw, nil
}

func copyReturnMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}

	copied := make(map[string]any, len(input))
	for key, value := range input {
		copied[key] = value
	}

	return copied
}
