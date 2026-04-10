package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/trigger"
)

type FlowNode struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
}

type FlowEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
}

type FlowData struct {
	Nodes []FlowNode `json:"nodes"`
	Edges []FlowEdge `json:"edges"`
}

type ExecutionState struct {
	PipelineID   string
	TriggerType  string
	Context      map[string]any
	NodeResults  map[string]*node.NodeResult
	NodeRuns     []NodeRun
	StartTime    time.Time
	Returned     bool
	ReturnNodeID string
	ReturnValue  any
}

type NodeRun struct {
	NodeID      string
	NodeType    string
	Input       map[string]any
	Result      *node.NodeResult
	StartedAt   time.Time
	CompletedAt time.Time
}

type NodeStart struct {
	NodeID    string
	NodeType  string
	Input     map[string]any
	StartedAt time.Time
}

type ExecutionObserver struct {
	OnNodeStarted   func(NodeStart)
	OnNodeCompleted func(NodeRun)
}

type routingDecision struct {
	condition *bool
	handles   map[string]bool
}

type Engine struct {
	registry *node.Registry
}

func NewEngine(registry *node.Registry) *Engine {
	return &Engine{
		registry: registry,
	}
}

func (e *Engine) Execute(ctx context.Context, flowData FlowData, triggerType string, observers ...*ExecutionObserver) (*ExecutionState, error) {
	return e.ExecuteWithInput(ctx, flowData, triggerType, nil, observers...)
}

func (e *Engine) ExecuteWithInput(ctx context.Context, flowData FlowData, triggerType string, executionContext map[string]any, observers ...*ExecutionObserver) (*ExecutionState, error) {
	var observer *ExecutionObserver
	if len(observers) > 0 {
		observer = observers[0]
	}

	state := &ExecutionState{
		TriggerType: triggerType,
		Context:     copyExecutionContext(executionContext),
		NodeResults: make(map[string]*node.NodeResult),
		NodeRuns:    make([]NodeRun, 0, len(flowData.Nodes)),
		StartTime:   time.Now(),
	}

	nodeMap := make(map[string]FlowNode)
	adjacency := make(map[string][]FlowEdge)
	inDegree := make(map[string]int)
	toolTargets := collectToolTargets(flowData.Edges)

	for _, n := range flowData.Nodes {
		nodeMap[n.ID] = n
		inDegree[n.ID] = 0
	}

	for _, edge := range flowData.Edges {
		if isToolEdge(edge) {
			continue
		}
		adjacency[edge.Source] = append(adjacency[edge.Source], edge)
		inDegree[edge.Target]++
	}

	runtime := &ExecutionRuntime{
		FlowData:  flowData,
		NodeMap:   nodeMap,
		Registry:  e.registry,
		State:     state,
		ToolNodes: toolTargets,
	}
	ctx = withExecutionRuntime(ctx, runtime)

	queue := e.buildInitialQueue(ctx, nodeMap, inDegree, triggerType, toolTargets)

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return state, err
		}

		currentID := queue[0]
		queue = queue[1:]

		currentNode := nodeMap[currentID]

		nodeType := node.NodeType(currentNode.Type)
		configData := currentNode.Data

		if len(currentNode.Data) > 0 {
			var nodeData map[string]json.RawMessage
			if err := json.Unmarshal(currentNode.Data, &nodeData); err == nil {
				if t, ok := nodeData["type"]; ok {
					var typeStr string
					if err := json.Unmarshal(t, &typeStr); err == nil && typeStr != "" {
						nodeType = node.NodeType(typeStr)
					}
				}
				if c, ok := nodeData["config"]; ok {
					configData = c
				}
			}
		}

		input := e.buildInput(currentID, flowData.Edges, state)
		startedAt := time.Now()

		executor, err := e.registry.Get(nodeType)
		if err != nil {
			result := &node.NodeResult{Error: err}
			continued := shouldContinueNodeError(nodeType, configData, err)
			if continued {
				result = buildContinuedErrorResult(currentID, nodeType, input, err)
			}

			e.recordNodeRun(state, currentID, string(nodeType), input, result, startedAt, time.Now(), observer)
			if continued {
				queue = e.enqueueOutgoingEdges(queue, adjacency[currentID], routingDecision{}, inDegree)
				continue
			}

			return state, fmt.Errorf("node %s: %w", currentID, err)
		}

		e.notifyNodeStarted(currentID, string(nodeType), input, startedAt, observer)

		result, err := executor.Execute(withCurrentNodeID(ctx, currentID), configData, input)
		completedAt := time.Now()
		if err != nil {
			result := &node.NodeResult{Error: err}
			continued := shouldContinueNodeError(nodeType, configData, err)
			if continued {
				result = buildContinuedErrorResult(currentID, nodeType, input, err)
			}

			e.recordNodeRun(state, currentID, string(nodeType), input, result, startedAt, completedAt, observer)
			if continued {
				queue = e.enqueueOutgoingEdges(queue, adjacency[currentID], routingDecision{}, inDegree)
				continue
			}

			return state, fmt.Errorf("execute node %s (%s): %w", currentID, nodeType, err)
		}

		if result == nil {
			result = &node.NodeResult{}
		}

		e.recordNodeRun(state, currentID, string(nodeType), input, result, startedAt, completedAt, observer)
		if result.ReturnValue != nil {
			state.Returned = true
			state.ReturnNodeID = currentID
			state.ReturnValue = result.ReturnValue
			return state, nil
		}

		routing := e.extractRoutingDecision(result)
		queue = e.enqueueOutgoingEdges(queue, adjacency[currentID], routing, inDegree)
	}

	return state, nil
}

func (e *Engine) recordNodeRun(
	state *ExecutionState,
	nodeID string,
	nodeType string,
	input map[string]any,
	result *node.NodeResult,
	startedAt time.Time,
	completedAt time.Time,
	observer *ExecutionObserver,
) {
	copiedInput := make(map[string]any, len(input))
	for key, value := range input {
		copiedInput[key] = value
	}

	state.NodeResults[nodeID] = result

	run := NodeRun{
		NodeID:      nodeID,
		NodeType:    nodeType,
		Input:       copiedInput,
		Result:      result,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}

	state.NodeRuns = append(state.NodeRuns, run)

	if observer != nil {
		if observer.OnNodeCompleted != nil {
			observer.OnNodeCompleted(run)
		}
	}
}

func (e *Engine) notifyNodeStarted(
	nodeID string,
	nodeType string,
	input map[string]any,
	startedAt time.Time,
	observer *ExecutionObserver,
) {
	if observer == nil || observer.OnNodeStarted == nil {
		return
	}

	copiedInput := make(map[string]any, len(input))
	for key, value := range input {
		copiedInput[key] = value
	}

	observer.OnNodeStarted(NodeStart{
		NodeID:    nodeID,
		NodeType:  nodeType,
		Input:     copiedInput,
		StartedAt: startedAt,
	})
}

func (e *Engine) extractRoutingDecision(result *node.NodeResult) routingDecision {
	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		return routingDecision{}
	}

	decision := routingDecision{}
	if val, ok := output["result"]; ok {
		if b, ok := val.(bool); ok {
			decision.condition = &b
		}
	}

	rawMatches, ok := output["matches"]
	if !ok {
		return decision
	}

	matchMap, ok := rawMatches.(map[string]any)
	if !ok {
		return decision
	}

	decision.handles = make(map[string]bool, len(matchMap))
	for handleID, rawValue := range matchMap {
		if matched, ok := rawValue.(bool); ok {
			decision.handles[handleID] = matched
		}
	}

	if len(decision.handles) == 0 {
		decision.handles = nil
	}

	return decision
}

func edgeMatchesCondition(edge FlowEdge, result bool) bool {
	handle := edge.SourceHandle
	if handle == "" {
		return true // no handle specified, follow all edges
	}
	return handle == fmt.Sprintf("%t", result)
}

func edgeMatchesHandle(edge FlowEdge, matches map[string]bool) bool {
	handle := edge.SourceHandle
	if handle == "" {
		return true
	}

	matched, ok := matches[handle]
	if !ok {
		return false
	}

	return matched
}

func (e *Engine) buildInput(nodeID string, edges []FlowEdge, state *ExecutionState) map[string]any {
	input := copyExecutionContext(state.Context)

	for _, edge := range edges {
		if isToolEdge(edge) {
			continue
		}
		if edge.Target == nodeID {
			if result, ok := state.NodeResults[edge.Source]; ok {
				var sourceOutput map[string]any
				if err := json.Unmarshal(result.Output, &sourceOutput); err == nil {
					for k, v := range sourceOutput {
						input[k] = v
					}
				}
			}
		}
	}

	return input
}

func (e *Engine) buildInitialQueue(ctx context.Context, nodeMap map[string]FlowNode, inDegree map[string]int, triggerType string, toolTargets map[string]struct{}) []string {
	queue := make([]string, 0, len(inDegree))
	rootIDs := make([]string, 0, len(inDegree))
	hasRootTrigger := false

	for id, degree := range inDegree {
		if degree != 0 {
			continue
		}
		if _, isToolNode := toolTargets[id]; isToolNode {
			continue
		}

		flowNode := nodeMap[id]
		nodeType, config := decodeNodeTypeAndConfig(flowNode)
		if isToolNodeType(nodeType) || isVisualNodeType(nodeType) {
			continue
		}

		rootIDs = append(rootIDs, id)
		if trigger.IsTriggerType(nodeType) {
			hasRootTrigger = true
			if trigger.MatchesExecution(ctx, nodeType, config, triggerType) {
				queue = append(queue, id)
			}
		}
	}

	if hasRootTrigger {
		return queue
	}

	if triggerType != "manual" {
		return nil
	}

	return rootIDs
}

func (e *Engine) enqueueOutgoingEdges(queue []string, edges []FlowEdge, routing routingDecision, inDegree map[string]int) []string {
	for _, edge := range edges {
		if routing.condition != nil {
			if !edgeMatchesCondition(edge, *routing.condition) {
				continue
			}
		}

		if routing.handles != nil {
			if !edgeMatchesHandle(edge, routing.handles) {
				continue
			}
		}

		inDegree[edge.Target]--
		if inDegree[edge.Target] == 0 {
			queue = append(queue, edge.Target)
		}
	}

	return queue
}

func isToolEdge(edge FlowEdge) bool {
	return edge.SourceHandle == "tool"
}

func isToolNodeType(nodeType node.NodeType) bool {
	return len(nodeType) >= 5 && string(nodeType[:5]) == "tool:"
}

func collectToolTargets(edges []FlowEdge) map[string]struct{} {
	targets := make(map[string]struct{})

	for _, edge := range edges {
		if !isToolEdge(edge) {
			continue
		}
		targets[edge.Target] = struct{}{}
	}

	return targets
}

func copyExecutionContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}

	copied := make(map[string]any, len(input))
	for key, value := range input {
		copied[key] = value
	}

	return copied
}

func decodeNodeTypeAndConfig(flowNode FlowNode) (node.NodeType, json.RawMessage) {
	nodeType := node.NodeType(flowNode.Type)
	configData := flowNode.Data

	if len(flowNode.Data) == 0 {
		return nodeType, configData
	}

	var nodeData map[string]json.RawMessage
	if err := json.Unmarshal(flowNode.Data, &nodeData); err != nil {
		return nodeType, configData
	}

	if t, ok := nodeData["type"]; ok {
		var typeStr string
		if err := json.Unmarshal(t, &typeStr); err == nil && typeStr != "" {
			nodeType = node.NodeType(typeStr)
		}
	}
	if c, ok := nodeData["config"]; ok {
		configData = c
	}

	return nodeType, configData
}
