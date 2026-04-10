package pipeline

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
)

type runtimeContextKey struct{}
type currentNodeContextKey struct{}

type ExecutionRuntime struct {
	FlowData  FlowData
	NodeMap   map[string]FlowNode
	Registry  *node.Registry
	State     *ExecutionState
	ToolNodes map[string]struct{}
}

type IncomingNodeOutput struct {
	NodeID       string
	NodeType     string
	Label        string
	SourceHandle string
	Output       any
}

func withExecutionRuntime(ctx context.Context, runtime *ExecutionRuntime) context.Context {
	return context.WithValue(ctx, runtimeContextKey{}, runtime)
}

func RuntimeFromContext(ctx context.Context) *ExecutionRuntime {
	runtime, _ := ctx.Value(runtimeContextKey{}).(*ExecutionRuntime)
	return runtime
}

func withCurrentNodeID(ctx context.Context, nodeID string) context.Context {
	return context.WithValue(ctx, currentNodeContextKey{}, nodeID)
}

func CurrentNodeIDFromContext(ctx context.Context) string {
	nodeID, _ := ctx.Value(currentNodeContextKey{}).(string)
	return nodeID
}

func (r *ExecutionRuntime) ConnectedToolNodes(nodeID string) []FlowNode {
	if r == nil {
		return nil
	}

	connected := make([]FlowNode, 0)
	for _, edge := range r.FlowData.Edges {
		if edge.Source != nodeID || !isToolEdge(edge) {
			continue
		}

		if toolNode, ok := r.NodeMap[edge.Target]; ok {
			connected = append(connected, toolNode)
		}
	}

	return connected
}

func (r *ExecutionRuntime) IncomingNodeOutputs(nodeID string) []IncomingNodeOutput {
	if r == nil || r.State == nil {
		return nil
	}

	incoming := make([]IncomingNodeOutput, 0)
	for _, edge := range r.FlowData.Edges {
		if isToolEdge(edge) || edge.Target != nodeID {
			continue
		}

		result, ok := r.State.NodeResults[edge.Source]
		if !ok || result == nil {
			continue
		}

		flowNode, ok := r.NodeMap[edge.Source]
		if !ok {
			continue
		}

		incoming = append(incoming, IncomingNodeOutput{
			NodeID:       edge.Source,
			NodeType:     string(decodeRuntimeNodeType(flowNode)),
			Label:        decodeRuntimeNodeLabel(flowNode),
			SourceHandle: edge.SourceHandle,
			Output:       decodeRuntimeNodeOutput(result.Output),
		})
	}

	return incoming
}

func decodeRuntimeNodeType(flowNode FlowNode) node.NodeType {
	nodeType, _ := decodeNodeTypeAndConfig(flowNode)
	return nodeType
}

func decodeRuntimeNodeLabel(flowNode FlowNode) string {
	if len(flowNode.Data) > 0 {
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(flowNode.Data, &payload); err == nil {
			if rawLabel, ok := payload["label"]; ok {
				var label string
				if err := json.Unmarshal(rawLabel, &label); err == nil && strings.TrimSpace(label) != "" {
					return strings.TrimSpace(label)
				}
			}
		}
	}

	return flowNode.ID
}

func decodeRuntimeNodeOutput(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return decoded
	}

	return string(raw)
}
