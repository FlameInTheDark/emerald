package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
)

const (
	nodeErrorPolicyStop     = "stop"
	nodeErrorPolicyContinue = "continue"
)

func nodeSupportsErrorPolicy(nodeType node.NodeType) bool {
	trimmed := strings.TrimSpace(string(nodeType))

	switch {
	case trimmed == "":
		return true
	case strings.HasPrefix(trimmed, "tool:"):
		return false
	case trimmed == "visual:group":
		return false
	case trimmed == string(node.TypeLogicReturn):
		return false
	case trimmed == string(node.TypeLogicCondition):
		return false
	case trimmed == string(node.TypeLogicSwitch):
		return false
	default:
		return true
	}
}

func decodeNodeErrorPolicy(config json.RawMessage) string {
	if len(config) == 0 {
		return nodeErrorPolicyStop
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(config, &payload); err != nil {
		return nodeErrorPolicyStop
	}

	rawPolicy, ok := payload["errorPolicy"]
	if !ok {
		return nodeErrorPolicyStop
	}

	var policy string
	if err := json.Unmarshal(rawPolicy, &policy); err != nil {
		return nodeErrorPolicyStop
	}

	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", nodeErrorPolicyStop:
		return nodeErrorPolicyStop
	case nodeErrorPolicyContinue:
		return nodeErrorPolicyContinue
	default:
		return strings.ToLower(strings.TrimSpace(policy))
	}
}

func validateNodeErrorPolicy(nodeType node.NodeType, config json.RawMessage) error {
	policy := decodeNodeErrorPolicy(config)
	if policy == nodeErrorPolicyStop {
		return nil
	}

	if policy != nodeErrorPolicyContinue {
		return fmt.Errorf("errorPolicy must be %q or %q", nodeErrorPolicyStop, nodeErrorPolicyContinue)
	}

	if !nodeSupportsErrorPolicy(nodeType) {
		return fmt.Errorf("errorPolicy %q is not supported for node type %s", nodeErrorPolicyContinue, nodeType)
	}

	return nil
}

func shouldContinueNodeError(nodeType node.NodeType, config json.RawMessage, err error) bool {
	if err == nil || isCancellationError(err) {
		return false
	}

	return nodeSupportsErrorPolicy(nodeType) && decodeNodeErrorPolicy(config) == nodeErrorPolicyContinue
}

func buildContinuedErrorResult(nodeID string, nodeType node.NodeType, input map[string]any, err error) *node.NodeResult {
	output := copyExecutionContext(input)
	message := err.Error()

	output["failed"] = true
	output["error"] = message
	output["errorMessage"] = message
	output["errorNodeId"] = nodeID
	output["errorNodeType"] = string(nodeType)

	data, _ := json.Marshal(output)

	return &node.NodeResult{
		Output: data,
		Error:  err,
	}
}
