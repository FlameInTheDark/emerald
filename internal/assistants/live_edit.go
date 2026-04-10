package assistants

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/pipeline"
	"github.com/FlameInTheDark/emerald/internal/pipelineops"
)

type FlowValidator interface {
	ValidateFlowData(ctx context.Context, flowData pipeline.FlowData, allowUnavailablePlugins bool) error
}

type PipelineSnapshot struct {
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Status      string           `json:"status,omitempty"`
	Nodes       []map[string]any `json:"nodes"`
	Edges       []map[string]any `json:"edges"`
	Viewport    map[string]any   `json:"viewport,omitempty"`
}

type SelectionSnapshot struct {
	SelectedNodeID  string   `json:"selected_node_id,omitempty"`
	SelectedNodeIDs []string `json:"selected_node_ids,omitempty"`
}

type LivePipelineOperation struct {
	Type     string           `json:"type"`
	Nodes    []map[string]any `json:"nodes,omitempty"`
	Edges    []map[string]any `json:"edges,omitempty"`
	NodeIDs  []string         `json:"node_ids,omitempty"`
	EdgeIDs  []string         `json:"edge_ids,omitempty"`
	Viewport map[string]any   `json:"viewport,omitempty"`
}

func NormalizePipelineSnapshot(snapshot PipelineSnapshot) (PipelineSnapshot, error) {
	nodes, err := cloneJSONValue(snapshot.Nodes)
	if err != nil {
		return PipelineSnapshot{}, fmt.Errorf("clone pipeline nodes: %w", err)
	}
	edges, err := cloneJSONValue(snapshot.Edges)
	if err != nil {
		return PipelineSnapshot{}, fmt.Errorf("clone pipeline edges: %w", err)
	}

	normalized := PipelineSnapshot{
		Name:        strings.TrimSpace(snapshot.Name),
		Description: strings.TrimSpace(snapshot.Description),
		Status:      strings.TrimSpace(snapshot.Status),
		Nodes:       nodes,
		Edges:       edges,
	}

	if snapshot.Viewport != nil {
		viewport, err := normalizeViewport(snapshot.Viewport)
		if err != nil {
			return PipelineSnapshot{}, err
		}
		normalized.Viewport = viewport
	}

	if normalized.Nodes == nil {
		normalized.Nodes = []map[string]any{}
	}
	if normalized.Edges == nil {
		normalized.Edges = []map[string]any{}
	}

	return normalized, nil
}

func ValidateAndApplyOperations(
	snapshot PipelineSnapshot,
	operations []LivePipelineOperation,
	validators ...FlowValidator,
) ([]LivePipelineOperation, PipelineSnapshot, error) {
	working, err := NormalizePipelineSnapshot(snapshot)
	if err != nil {
		return nil, PipelineSnapshot{}, err
	}
	if len(operations) == 0 {
		return nil, PipelineSnapshot{}, fmt.Errorf("at least one live pipeline operation is required")
	}

	normalized := make([]LivePipelineOperation, 0, len(operations))
	for _, operation := range operations {
		nextOperation, nextSnapshot, err := applyOperation(working, operation)
		if err != nil {
			return nil, PipelineSnapshot{}, err
		}
		working = nextSnapshot
		normalized = append(normalized, nextOperation)
	}

	var validator FlowValidator
	if len(validators) > 0 {
		validator = validators[0]
	}

	if err := validateSnapshot(working, validator); err != nil {
		return nil, PipelineSnapshot{}, err
	}

	return normalized, working, nil
}

func applyOperation(snapshot PipelineSnapshot, operation LivePipelineOperation) (LivePipelineOperation, PipelineSnapshot, error) {
	working, err := NormalizePipelineSnapshot(snapshot)
	if err != nil {
		return LivePipelineOperation{}, PipelineSnapshot{}, err
	}

	switch strings.TrimSpace(operation.Type) {
	case "add_nodes":
		nodes, err := normalizeNodes(operation.Nodes, true)
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		existing := indexByID(working.Nodes)
		for _, node := range nodes {
			id, _ := extractID(node, "node")
			if _, ok := existing[id]; ok {
				return LivePipelineOperation{}, PipelineSnapshot{}, fmt.Errorf("node %q already exists", id)
			}
			working.Nodes = append(working.Nodes, node)
		}
		return LivePipelineOperation{Type: "add_nodes", Nodes: nodes}, working, nil
	case "update_nodes":
		nodes, err := normalizeUpdatedNodes(working.Nodes, operation.Nodes)
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		byID := indexByID(working.Nodes)
		for _, node := range nodes {
			id, _ := extractID(node, "node")
			index, ok := byID[id]
			if !ok {
				return LivePipelineOperation{}, PipelineSnapshot{}, fmt.Errorf("node %q was not found", id)
			}
			working.Nodes[index] = node
		}
		return LivePipelineOperation{Type: "update_nodes", Nodes: nodes}, working, nil
	case "delete_nodes":
		nodeIDs, err := normalizeStringIDs(operation.NodeIDs, "node_ids")
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		toDelete := make(map[string]struct{}, len(nodeIDs))
		for _, id := range nodeIDs {
			toDelete[id] = struct{}{}
		}
		if err := ensureIDsExist(indexByID(working.Nodes), nodeIDs, "node"); err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}

		filteredNodes := make([]map[string]any, 0, len(working.Nodes)-len(nodeIDs))
		for _, node := range working.Nodes {
			id, _ := extractID(node, "node")
			if _, ok := toDelete[id]; ok {
				continue
			}
			filteredNodes = append(filteredNodes, node)
		}

		filteredEdges := make([]map[string]any, 0, len(working.Edges))
		for _, edge := range working.Edges {
			source, err := extractRequiredString(edge, "source", "edge")
			if err != nil {
				return LivePipelineOperation{}, PipelineSnapshot{}, err
			}
			target, err := extractRequiredString(edge, "target", "edge")
			if err != nil {
				return LivePipelineOperation{}, PipelineSnapshot{}, err
			}
			if _, ok := toDelete[source]; ok {
				continue
			}
			if _, ok := toDelete[target]; ok {
				continue
			}
			filteredEdges = append(filteredEdges, edge)
		}

		working.Nodes = filteredNodes
		working.Edges = filteredEdges
		return LivePipelineOperation{Type: "delete_nodes", NodeIDs: nodeIDs}, working, nil
	case "add_edges":
		edges, err := normalizeEdges(operation.Edges, true)
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		existing := indexByID(working.Edges)
		for _, edge := range edges {
			id, _ := extractID(edge, "edge")
			if _, ok := existing[id]; ok {
				return LivePipelineOperation{}, PipelineSnapshot{}, fmt.Errorf("edge %q already exists", id)
			}
			working.Edges = append(working.Edges, edge)
		}
		return LivePipelineOperation{Type: "add_edges", Edges: edges}, working, nil
	case "update_edges":
		edges, err := normalizeUpdatedEdges(working.Edges, operation.Edges)
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		byID := indexByID(working.Edges)
		for _, edge := range edges {
			id, _ := extractID(edge, "edge")
			index, ok := byID[id]
			if !ok {
				return LivePipelineOperation{}, PipelineSnapshot{}, fmt.Errorf("edge %q was not found", id)
			}
			working.Edges[index] = edge
		}
		return LivePipelineOperation{Type: "update_edges", Edges: edges}, working, nil
	case "delete_edges":
		edgeIDs, err := normalizeStringIDs(operation.EdgeIDs, "edge_ids")
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		if err := ensureIDsExist(indexByID(working.Edges), edgeIDs, "edge"); err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		toDelete := make(map[string]struct{}, len(edgeIDs))
		for _, id := range edgeIDs {
			toDelete[id] = struct{}{}
		}
		filtered := make([]map[string]any, 0, len(working.Edges)-len(edgeIDs))
		for _, edge := range working.Edges {
			id, _ := extractID(edge, "edge")
			if _, ok := toDelete[id]; ok {
				continue
			}
			filtered = append(filtered, edge)
		}
		working.Edges = filtered
		return LivePipelineOperation{Type: "delete_edges", EdgeIDs: edgeIDs}, working, nil
	case "set_viewport":
		viewport, err := normalizeViewport(operation.Viewport)
		if err != nil {
			return LivePipelineOperation{}, PipelineSnapshot{}, err
		}
		working.Viewport = viewport
		return LivePipelineOperation{Type: "set_viewport", Viewport: viewport}, working, nil
	default:
		return LivePipelineOperation{}, PipelineSnapshot{}, fmt.Errorf("unsupported live pipeline operation %q", strings.TrimSpace(operation.Type))
	}
}

func validateSnapshot(snapshot PipelineSnapshot, validator FlowValidator) error {
	nodesJSON, err := json.Marshal(snapshot.Nodes)
	if err != nil {
		return fmt.Errorf("encode nodes: %w", err)
	}
	edgesJSON, err := json.Marshal(snapshot.Edges)
	if err != nil {
		return fmt.Errorf("encode edges: %w", err)
	}
	if err := pipelineops.ValidateDefinition(string(nodesJSON), string(edgesJSON)); err != nil {
		return fmt.Errorf("invalid live pipeline edit: %w", err)
	}
	if validator != nil {
		flowData, err := pipeline.ParseFlowData(string(nodesJSON), string(edgesJSON))
		if err != nil {
			return fmt.Errorf("parse live pipeline edit: %w", err)
		}
		if err := validator.ValidateFlowData(context.Background(), *flowData, true); err != nil {
			return fmt.Errorf("invalid live pipeline edit: %w", err)
		}
	}
	return nil
}

func normalizeNodes(nodes []map[string]any, requirePosition bool) ([]map[string]any, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("nodes are required")
	}

	normalized, err := cloneJSONValue(nodes)
	if err != nil {
		return nil, fmt.Errorf("clone nodes: %w", err)
	}
	seen := make(map[string]struct{}, len(normalized))
	for index := range normalized {
		id, err := extractID(normalized[index], "node")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate node id %q in operation payload", id)
		}
		seen[id] = struct{}{}
		normalized[index]["id"] = id
		if err := normalizeNodeForCanvas(normalized[index], requirePosition); err != nil {
			return nil, err
		}
	}
	return normalized, nil
}

func normalizeUpdatedNodes(existing []map[string]any, patches []map[string]any) ([]map[string]any, error) {
	if len(patches) == 0 {
		return nil, fmt.Errorf("nodes are required")
	}

	normalizedPatches, err := cloneJSONValue(patches)
	if err != nil {
		return nil, fmt.Errorf("clone nodes: %w", err)
	}

	byID := indexByID(existing)
	normalized := make([]map[string]any, 0, len(normalizedPatches))
	seen := make(map[string]struct{}, len(normalizedPatches))

	for index := range normalizedPatches {
		id, err := extractID(normalizedPatches[index], "node")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate node id %q in operation payload", id)
		}
		seen[id] = struct{}{}

		existingIndex, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("node %q was not found", id)
		}

		merged, err := mergeJSONObjects(existing[existingIndex], normalizedPatches[index])
		if err != nil {
			return nil, fmt.Errorf("merge node %q update: %w", id, err)
		}
		if err := normalizeNodeForCanvas(merged, false); err != nil {
			return nil, err
		}
		normalized = append(normalized, merged)
	}

	return normalized, nil
}

func normalizeEdges(edges []map[string]any, requireEndpoints bool) ([]map[string]any, error) {
	if len(edges) == 0 {
		return nil, fmt.Errorf("edges are required")
	}

	normalized, err := cloneJSONValue(edges)
	if err != nil {
		return nil, fmt.Errorf("clone edges: %w", err)
	}
	seen := make(map[string]struct{}, len(normalized))
	for index := range normalized {
		id, err := extractID(normalized[index], "edge")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate edge id %q in operation payload", id)
		}
		seen[id] = struct{}{}
		normalized[index]["id"] = id
		if requireEndpoints {
			source, err := extractRequiredString(normalized[index], "source", "edge")
			if err != nil {
				return nil, err
			}
			target, err := extractRequiredString(normalized[index], "target", "edge")
			if err != nil {
				return nil, err
			}
			normalized[index]["source"] = source
			normalized[index]["target"] = target
		}
		if handle, ok := normalized[index]["sourceHandle"]; ok {
			handleValue, ok := handle.(string)
			if !ok {
				return nil, fmt.Errorf("edge %q sourceHandle must be a string", id)
			}
			normalized[index]["sourceHandle"] = strings.TrimSpace(handleValue)
		}
	}
	return normalized, nil
}

func normalizeUpdatedEdges(existing []map[string]any, patches []map[string]any) ([]map[string]any, error) {
	if len(patches) == 0 {
		return nil, fmt.Errorf("edges are required")
	}

	normalizedPatches, err := normalizeEdges(patches, false)
	if err != nil {
		return nil, err
	}

	byID := indexByID(existing)
	normalized := make([]map[string]any, 0, len(normalizedPatches))

	for _, patch := range normalizedPatches {
		id, _ := extractID(patch, "edge")
		existingIndex, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("edge %q was not found", id)
		}

		merged, err := mergeJSONObjects(existing[existingIndex], patch)
		if err != nil {
			return nil, fmt.Errorf("merge edge %q update: %w", id, err)
		}

		source, err := extractRequiredString(merged, "source", "edge")
		if err != nil {
			return nil, err
		}
		target, err := extractRequiredString(merged, "target", "edge")
		if err != nil {
			return nil, err
		}
		merged["source"] = source
		merged["target"] = target
		normalized = append(normalized, merged)
	}

	return normalized, nil
}

func normalizeViewport(viewport map[string]any) (map[string]any, error) {
	if len(viewport) == 0 {
		return nil, fmt.Errorf("viewport is required")
	}
	x, err := extractRequiredNumber(viewport, "x", "viewport")
	if err != nil {
		return nil, err
	}
	y, err := extractRequiredNumber(viewport, "y", "viewport")
	if err != nil {
		return nil, err
	}
	zoom, err := extractRequiredNumber(viewport, "zoom", "viewport")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"x":    x,
		"y":    y,
		"zoom": zoom,
	}, nil
}

func normalizeStringIDs(values []string, field string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%s are required", field)
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("%s must not contain empty values", field)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized, nil
}

func ensureIDsExist(existing map[string]int, ids []string, label string) error {
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			return fmt.Errorf("%s %q was not found", label, id)
		}
	}
	return nil
}

func indexByID(items []map[string]any) map[string]int {
	index := make(map[string]int, len(items))
	for itemIndex, item := range items {
		id, err := extractID(item, "item")
		if err != nil {
			continue
		}
		index[id] = itemIndex
	}
	return index
}

func extractID(item map[string]any, label string) (string, error) {
	return extractRequiredString(item, "id", label)
}

func extractRequiredString(item map[string]any, field string, label string) (string, error) {
	raw, ok := item[field]
	if !ok {
		return "", fmt.Errorf("%s %s is required", label, field)
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s %s must be a string", label, field)
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s %s must not be empty", label, field)
	}
	return trimmed, nil
}

func extractRequiredNumber(item map[string]any, field string, label string) (float64, error) {
	raw, ok := item[field]
	if !ok {
		return 0, fmt.Errorf("%s %s is required", label, field)
	}
	switch typed := raw.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	default:
		return 0, fmt.Errorf("%s %s must be a number", label, field)
	}
}

func normalizeNodeForCanvas(node map[string]any, requirePosition bool) error {
	if rawType, ok := node["type"]; ok {
		typeValue, ok := rawType.(string)
		if !ok {
			return fmt.Errorf("node type must be a string")
		}
		trimmedType := strings.TrimSpace(typeValue)
		if trimmedType == "" {
			return fmt.Errorf("node type must not be empty")
		}
		node["type"] = trimmedType
	} else {
		node["type"] = "emerald"
	}

	positionRaw, hasPosition := node["position"]
	if !hasPosition {
		if requirePosition {
			return fmt.Errorf("node position is required")
		}
		return nil
	}

	position, ok := positionRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("node position must be an object")
	}
	x, err := extractRequiredNumber(position, "x", "node position")
	if err != nil {
		return err
	}
	y, err := extractRequiredNumber(position, "y", "node position")
	if err != nil {
		return err
	}
	node["position"] = map[string]any{
		"x": x,
		"y": y,
	}
	return nil
}

func mergeJSONObjects(base map[string]any, patch map[string]any) (map[string]any, error) {
	merged, err := cloneJSONValue(base)
	if err != nil {
		return nil, err
	}

	for key, patchValue := range patch {
		if patchMap, ok := patchValue.(map[string]any); ok {
			if baseMap, ok := merged[key].(map[string]any); ok {
				nested, err := mergeJSONObjects(baseMap, patchMap)
				if err != nil {
					return nil, err
				}
				merged[key] = nested
				continue
			}
		}

		merged[key] = patchValue
	}

	return merged, nil
}

func cloneJSONValue[T any](value T) (T, error) {
	var cloned T
	encoded, err := json.Marshal(value)
	if err != nil {
		return cloned, err
	}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return cloned, err
	}
	return cloned, nil
}
