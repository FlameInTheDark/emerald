package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/nodeconfig"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
)

type mergeConfig struct {
	Mode string `json:"mode"`
}

type MergeNode struct{}

func (e *MergeNode) Execute(ctx context.Context, config json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	var cfg mergeConfig
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "shallow"
	}
	if cfg.Mode != "shallow" && cfg.Mode != "deep" {
		return nil, fmt.Errorf("mode must be either shallow or deep")
	}

	incoming, err := incomingOutputsFromContext(ctx)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]any)
	extras := make(map[string]any)
	entries := make([]map[string]any, 0, len(incoming))

	for _, source := range incoming {
		entries = append(entries, source.asEntry(source.NodeID))

		objectValue, ok := source.Output.(map[string]any)
		if !ok {
			extras[source.NodeID] = source.Output
			continue
		}

		if cfg.Mode == "deep" {
			merged = deepMergeMaps(merged, objectValue)
			continue
		}

		for key, value := range objectValue {
			merged[key] = value
		}
	}

	output := cloneMap(merged)
	output["merged"] = cloneMap(merged)
	output["mode"] = cfg.Mode
	output["count"] = len(incoming)
	output["entries"] = entries
	if len(extras) > 0 {
		output["extras"] = extras
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *MergeNode) Validate(config json.RawMessage) error {
	var cfg mergeConfig
	if len(config) == 0 {
		return nil
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		return nil
	}
	if cfg.Mode != "shallow" && cfg.Mode != "deep" {
		return fmt.Errorf("mode must be either shallow or deep")
	}
	return nil
}

type AggregateNode struct{}

func (e *AggregateNode) Execute(ctx context.Context, config json.RawMessage, _ map[string]any) (*node.NodeResult, error) {
	cfg, err := nodeconfig.ParseAggregateConfig(config)
	if err != nil {
		return nil, err
	}

	incoming, err := incomingOutputsFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := cfg.ValidateResolvedNodeIDs(incomingSourceNodeIDs(incoming)); err != nil {
		return nil, err
	}

	items := make([]any, 0, len(incoming))
	entries := make([]map[string]any, 0, len(incoming))
	byNodeID := make(map[string]any, len(incoming))

	for _, source := range incoming {
		resolvedNodeID := cfg.ResolveNodeID(source.NodeID)
		items = append(items, source.Output)
		entries = append(entries, source.asEntry(resolvedNodeID))
		byNodeID[resolvedNodeID] = source.Output
	}

	output := map[string]any{
		"count":    len(incoming),
		"items":    items,
		"entries":  entries,
		"byNodeId": byNodeID,
	}

	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *AggregateNode) Validate(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}

	if _, err := nodeconfig.ParseAggregateConfig(config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}

type incomingSource struct {
	NodeID       string
	NodeType     string
	Label        string
	SourceHandle string
	Output       any
}

func incomingOutputsFromContext(ctx context.Context) ([]incomingSource, error) {
	runtime := pipeline.RuntimeFromContext(ctx)
	if runtime == nil {
		return nil, fmt.Errorf("execution runtime is not available")
	}

	currentNodeID := pipeline.CurrentNodeIDFromContext(ctx)
	if strings.TrimSpace(currentNodeID) == "" {
		return nil, fmt.Errorf("current node is not available")
	}

	upstream := runtime.IncomingNodeOutputs(currentNodeID)
	result := make([]incomingSource, 0, len(upstream))
	for _, source := range upstream {
		result = append(result, incomingSource{
			NodeID:       source.NodeID,
			NodeType:     source.NodeType,
			Label:        source.Label,
			SourceHandle: source.SourceHandle,
			Output:       source.Output,
		})
	}

	return result, nil
}

func (s incomingSource) asEntry(nodeID string) map[string]any {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		nodeID = s.NodeID
	}

	entry := map[string]any{
		"nodeId":   nodeID,
		"nodeType": s.NodeType,
		"label":    s.Label,
		"data":     s.Output,
	}
	if nodeID != s.NodeID {
		entry["originalNodeId"] = s.NodeID
	}
	if strings.TrimSpace(s.SourceHandle) != "" {
		entry["sourceHandle"] = s.SourceHandle
	}
	return entry
}

func incomingSourceNodeIDs(incoming []incomingSource) []string {
	sourceNodeIDs := make([]string, 0, len(incoming))
	for _, source := range incoming {
		sourceNodeIDs = append(sourceNodeIDs, source.NodeID)
	}

	return sourceNodeIDs
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func deepMergeMaps(base map[string]any, incoming map[string]any) map[string]any {
	merged := cloneMap(base)
	for key, incomingValue := range incoming {
		currentValue, exists := merged[key]
		if !exists {
			merged[key] = incomingValue
			continue
		}

		currentMap, currentIsMap := currentValue.(map[string]any)
		incomingMap, incomingIsMap := incomingValue.(map[string]any)
		if currentIsMap && incomingIsMap {
			merged[key] = deepMergeMaps(currentMap, incomingMap)
			continue
		}

		merged[key] = incomingValue
	}

	return merged
}
