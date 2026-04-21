package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/trigger"
	"github.com/FlameInTheDark/emerald/internal/nodeconfig"
)

func ValidateFlowData(flowData FlowData) error {
	if err := validateReturnNodes(flowData); err != nil {
		return err
	}
	if err := validateRootNodes(flowData); err != nil {
		return err
	}

	return validateFlowEdges(flowData)
}

func validateReturnNodes(flowData FlowData) error {
	count := 0

	for _, flowNode := range flowData.Nodes {
		nodeType, _ := decodeNodeTypeAndConfig(flowNode)
		if isVisualNodeType(nodeType) {
			continue
		}
		if nodeType == node.TypeLogicReturn {
			count++
		}
	}

	if count > 1 {
		return fmt.Errorf("only one Return node is allowed per pipeline")
	}

	return nil
}

func validateRootNodes(flowData FlowData) error {
	rootIDs, nodeMap := rootExecutableNodeIDs(flowData)
	if len(rootIDs) == 0 {
		return nil
	}

	manualTriggerRoots := 0
	for _, nodeID := range rootIDs {
		flowNode, ok := nodeMap[nodeID]
		if !ok {
			continue
		}

		nodeType, _ := decodeNodeTypeAndConfig(flowNode)
		if !trigger.IsTriggerType(nodeType) {
			return fmt.Errorf(
				"root node %q (%s) must be a trigger node; connect executable nodes from a trigger instead",
				nodeID,
				nodeType,
			)
		}

		if nodeType == node.TypeTriggerManual {
			manualTriggerRoots++
		}
	}

	if manualTriggerRoots > 1 {
		return fmt.Errorf("only one manual trigger root is allowed per pipeline")
	}

	return nil
}

func validateFlowEdges(flowData FlowData) error {
	nodeTypes := make(map[string]node.NodeType, len(flowData.Nodes))
	nodeConfigs := make(map[string]json.RawMessage, len(flowData.Nodes))

	for _, flowNode := range flowData.Nodes {
		nodeID := strings.TrimSpace(flowNode.ID)
		if nodeID == "" {
			return fmt.Errorf("all nodes must have an id")
		}
		if _, exists := nodeTypes[nodeID]; exists {
			return fmt.Errorf("duplicate node id %q", nodeID)
		}

		nodeType, _ := decodeNodeTypeAndConfig(flowNode)
		if err := validateNodeErrorPolicy(nodeType, decodeNodeConfig(flowNode)); err != nil {
			return fmt.Errorf("node %q: %w", nodeID, err)
		}
		nodeTypes[nodeID] = nodeType
		nodeConfigs[nodeID] = decodeNodeConfig(flowNode)
	}

	for _, edge := range flowData.Edges {
		if err := validateFlowEdge(edge, nodeTypes); err != nil {
			return err
		}
	}

	if err := validateAggregateNodeIDOverrides(flowData, nodeTypes, nodeConfigs); err != nil {
		return err
	}

	return nil
}

func validateAggregateNodeIDOverrides(flowData FlowData, nodeTypes map[string]node.NodeType, nodeConfigs map[string]json.RawMessage) error {
	incomingByTarget := make(map[string][]string)
	for _, edge := range flowData.Edges {
		if isToolEdge(edge) {
			continue
		}

		targetID := strings.TrimSpace(edge.Target)
		sourceID := strings.TrimSpace(edge.Source)
		if targetID == "" || sourceID == "" {
			continue
		}

		incomingByTarget[targetID] = append(incomingByTarget[targetID], sourceID)
	}

	for nodeID, nodeType := range nodeTypes {
		if nodeType != node.TypeLogicAggregate {
			continue
		}

		cfg, err := nodeconfig.ParseAggregateConfig(nodeConfigs[nodeID])
		if err != nil {
			return fmt.Errorf("node %q: invalid aggregate config: %w", nodeID, err)
		}

		if err := cfg.ValidateResolvedNodeIDs(incomingByTarget[nodeID]); err != nil {
			return fmt.Errorf("node %q: %w", nodeID, err)
		}
	}

	return nil
}

func validateFlowEdge(edge FlowEdge, nodeTypes map[string]node.NodeType) error {
	edgeID := strings.TrimSpace(edge.ID)
	if edgeID == "" {
		edgeID = fmt.Sprintf("%s->%s", edge.Source, edge.Target)
	}

	sourceID := strings.TrimSpace(edge.Source)
	targetID := strings.TrimSpace(edge.Target)
	if sourceID == "" || targetID == "" {
		return fmt.Errorf("edge %q must include source and target", edgeID)
	}

	sourceType, ok := nodeTypes[sourceID]
	if !ok {
		return fmt.Errorf("edge %q references unknown source node %q", edgeID, sourceID)
	}
	targetType, ok := nodeTypes[targetID]
	if !ok {
		return fmt.Errorf("edge %q references unknown target node %q", edgeID, targetID)
	}

	if isToolEdge(edge) {
		if sourceType != node.TypeLLMAgent {
			return fmt.Errorf("edge %q uses the tool handle, but source node %q is %q instead of %s", edgeID, sourceID, sourceType, node.TypeLLMAgent)
		}
		if !isToolNodeType(targetType) {
			return fmt.Errorf("edge %q uses the tool handle, but target node %q is %q instead of a tool node", edgeID, targetID, targetType)
		}
		return nil
	}

	if isVisualNodeType(sourceType) || isVisualNodeType(targetType) {
		nodeID := sourceID
		if isVisualNodeType(targetType) {
			nodeID = targetID
		}
		return fmt.Errorf("visual group node %q cannot have incoming or outgoing edges", nodeID)
	}
	if isToolNodeType(sourceType) {
		return fmt.Errorf("tool node %q (%s) cannot be part of the main execution chain; connect it from an LLM Agent tool handle instead", sourceID, sourceType)
	}
	if isToolNodeType(targetType) {
		return fmt.Errorf("tool node %q (%s) cannot be part of the main execution chain; connect it from an LLM Agent tool handle instead", targetID, targetType)
	}
	if sourceType == node.TypeLogicReturn {
		return fmt.Errorf("return node %q cannot have outgoing edges", sourceID)
	}
	if strings.HasPrefix(strings.TrimSpace(string(targetType)), "trigger:") {
		return fmt.Errorf("trigger node %q (%s) cannot have incoming edges", targetID, targetType)
	}

	return nil
}

func isVisualNodeType(nodeType node.NodeType) bool {
	return strings.TrimSpace(string(nodeType)) == "visual:group"
}

func decodeNodeConfig(flowNode FlowNode) json.RawMessage {
	_, config := decodeNodeTypeAndConfig(flowNode)
	return config
}
