package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FlameInTheDark/emerald/internal/node/trigger"
)

func ParseFlowData(nodesJSON string, edgesJSON string) (*FlowData, error) {
	var flowData FlowData
	if err := json.Unmarshal([]byte(nodesJSON), &flowData.Nodes); err != nil {
		return nil, fmt.Errorf("unmarshal nodes: %w", err)
	}
	if err := json.Unmarshal([]byte(edgesJSON), &flowData.Edges); err != nil {
		return nil, fmt.Errorf("unmarshal edges: %w", err)
	}
	return &flowData, nil
}

func HasMatchingRootTrigger(ctx context.Context, flowData FlowData, triggerType string) bool {
	inDegree := make(map[string]int, len(flowData.Nodes))
	nodeMap := make(map[string]FlowNode, len(flowData.Nodes))
	toolTargets := collectToolTargets(flowData.Edges)

	for _, node := range flowData.Nodes {
		inDegree[node.ID] = 0
		nodeMap[node.ID] = node
	}
	for _, edge := range flowData.Edges {
		if isToolEdge(edge) {
			continue
		}
		inDegree[edge.Target]++
	}

	hasRootTrigger := false
	for id, degree := range inDegree {
		if degree != 0 {
			continue
		}
		if _, isToolNode := toolTargets[id]; isToolNode {
			continue
		}

		nodeType, config := decodeNodeTypeAndConfig(nodeMap[id])
		if !trigger.IsTriggerType(nodeType) {
			continue
		}

		hasRootTrigger = true
		if trigger.MatchesExecution(ctx, nodeType, config, triggerType) {
			return true
		}
	}

	return !hasRootTrigger && triggerType == "manual"
}
