package pipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/logic"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
)

type testExecutor struct {
	name   string
	output map[string]any
}

func (e *testExecutor) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	out := make(map[string]any)
	for k, v := range e.output {
		out[k] = v
	}
	for k, v := range input {
		out["input_"+k] = v
	}
	data, _ := json.Marshal(out)
	return &node.NodeResult{Output: data}, nil
}

func (e *testExecutor) Validate(config json.RawMessage) error {
	return nil
}

type failingExecutor struct {
	err error
}

func (e *failingExecutor) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return nil, e.err
}

func (e *failingExecutor) Validate(config json.RawMessage) error {
	return nil
}

func TestEngine_Execute_SingleNode(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:node", &testExecutor{output: map[string]any{"result": "ok"}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "1", Type: "test:node"},
		},
		Edges: []pipeline.FlowEdge{},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(state.NodeResults) != 1 {
		t.Errorf("expected 1 node result, got %d", len(state.NodeResults))
	}

	if _, ok := state.NodeResults["1"]; !ok {
		t.Errorf("expected result for node '1'")
	}
}

func TestEngine_Execute_LinearPipeline(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:a", &testExecutor{output: map[string]any{"a_out": 1}})
	registry.Register("test:b", &testExecutor{output: map[string]any{"b_out": 2}})
	registry.Register("test:c", &testExecutor{output: map[string]any{"c_out": 3}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "1", Type: "test:a"},
			{ID: "2", Type: "test:b"},
			{ID: "3", Type: "test:c"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "1", Target: "2"},
			{ID: "e2", Source: "2", Target: "3"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(state.NodeResults) != 3 {
		t.Errorf("expected 3 node results, got %d", len(state.NodeResults))
	}

	for _, id := range []string{"1", "2", "3"} {
		if _, ok := state.NodeResults[id]; !ok {
			t.Errorf("expected result for node %q", id)
		}
	}
}

func TestEngine_Execute_ParallelBranches(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:start", &testExecutor{output: map[string]any{"start": true}})
	registry.Register("test:left", &testExecutor{output: map[string]any{"branch": "left"}})
	registry.Register("test:right", &testExecutor{output: map[string]any{"branch": "right"}})
	registry.Register("test:end", &testExecutor{output: map[string]any{"end": true}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "start", Type: "test:start"},
			{ID: "left", Type: "test:left"},
			{ID: "right", Type: "test:right"},
			{ID: "end", Type: "test:end"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "start", Target: "left"},
			{ID: "e2", Source: "start", Target: "right"},
			{ID: "e3", Source: "left", Target: "end"},
			{ID: "e4", Source: "right", Target: "end"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(state.NodeResults) != 4 {
		t.Errorf("expected 4 node results, got %d", len(state.NodeResults))
	}
}

func TestEngine_Execute_UnknownNodeType(t *testing.T) {
	registry := node.NewRegistry()
	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "1", Type: "unknown:type"},
		},
		Edges: []pipeline.FlowEdge{},
	}

	_, err := engine.Execute(context.Background(), flowData, "manual")
	if err == nil {
		t.Errorf("expected error for unknown node type, got nil")
	}
}

func TestEngine_Execute_FailedNodeIsRecorded(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:fail", &failingExecutor{err: errors.New("boom")})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "fail", Type: "test:fail"},
		},
		Edges: []pipeline.FlowEdge{},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err == nil {
		t.Fatalf("expected error for failing node, got nil")
	}

	result, ok := state.NodeResults["fail"]
	if !ok {
		t.Fatalf("expected failed node result to be recorded")
	}

	if result.Error == nil || result.Error.Error() != "boom" {
		t.Fatalf("expected failed node error to be recorded, got %#v", result.Error)
	}

	if len(state.NodeRuns) != 1 {
		t.Fatalf("expected 1 node run, got %d", len(state.NodeRuns))
	}

	if state.NodeRuns[0].NodeID != "fail" {
		t.Fatalf("expected failed node run to be tracked for 'fail', got %q", state.NodeRuns[0].NodeID)
	}
}

func TestEngine_Execute_ContinuesWhenNodeErrorPolicyIsContinue(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:fail", &failingExecutor{err: errors.New("boom")})
	registry.Register("test:consumer", &testExecutor{output: map[string]any{"handled": true}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{
				ID:   "fail",
				Type: "test:fail",
				Data: json.RawMessage(`{"config":{"errorPolicy":"continue"}}`),
			},
			{ID: "consumer", Type: "test:consumer"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "fail", Target: "consumer"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(state.NodeRuns) != 2 {
		t.Fatalf("expected 2 node runs, got %d", len(state.NodeRuns))
	}

	failResult := state.NodeResults["fail"]
	if failResult == nil || failResult.Error == nil {
		t.Fatalf("expected failed node result to be recorded")
	}

	consumerResult := state.NodeResults["consumer"]
	if consumerResult == nil {
		t.Fatalf("expected consumer result")
	}

	var output map[string]any
	if err := json.Unmarshal(consumerResult.Output, &output); err != nil {
		t.Fatalf("unmarshal consumer output: %v", err)
	}

	if got := output["input_handled"]; got != nil {
		t.Fatalf("expected no upstream handled flag before consumer output, got %#v", got)
	}
	if got := output["input_failed"]; got != true {
		t.Fatalf("input_failed = %#v, want true", got)
	}
	if got := output["input_error"]; got != "boom" {
		t.Fatalf("input_error = %#v, want boom", got)
	}
	if got := output["input_errorMessage"]; got != "boom" {
		t.Fatalf("input_errorMessage = %#v, want boom", got)
	}
	if got := output["input_errorNodeId"]; got != "fail" {
		t.Fatalf("input_errorNodeId = %#v, want fail", got)
	}
	if got := output["input_errorNodeType"]; got != "test:fail" {
		t.Fatalf("input_errorNodeType = %#v, want test:fail", got)
	}
}

func TestEngine_Execute_EmptyPipeline(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:node", &testExecutor{output: map[string]any{}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{},
		Edges: []pipeline.FlowEdge{},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(state.NodeResults) != 0 {
		t.Errorf("expected 0 node results for empty pipeline, got %d", len(state.NodeResults))
	}
}

func TestEngine_Execute_IgnoresVisualGroupRoots(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:node", &testExecutor{output: map[string]any{"result": "ok"}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "group-1", Data: json.RawMessage(`{"type":"visual:group"}`)},
			{ID: "node-1", Type: "test:node"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(state.NodeResults) != 1 {
		t.Fatalf("expected 1 node result, got %d", len(state.NodeResults))
	}
	if _, ok := state.NodeResults["node-1"]; !ok {
		t.Fatalf("expected runtime node to execute")
	}
	if _, ok := state.NodeResults["group-1"]; ok {
		t.Fatalf("did not expect visual group to execute")
	}
}

func TestEngine_Execute_DataFlow(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:producer", &testExecutor{output: map[string]any{"value": 42, "name": "test"}})
	registry.Register("test:consumer", &testExecutor{output: map[string]any{"consumed": true}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "producer", Type: "test:producer"},
			{ID: "consumer", Type: "test:consumer"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "producer", Target: "consumer"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	consumerResult := state.NodeResults["consumer"]
	var output map[string]any
	if err := json.Unmarshal(consumerResult.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if output["input_value"] != float64(42) {
		t.Errorf("consumer input_value = %v, want 42", output["input_value"])
	}
	if output["input_name"] != "test" {
		t.Errorf("consumer input_name = %v, want 'test'", output["input_name"])
	}
}

func TestEngine_Execute_StateFields(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:node", &testExecutor{output: map[string]any{}})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "1", Type: "test:node"},
		},
		Edges: []pipeline.FlowEdge{},
	}

	state, err := engine.Execute(context.Background(), flowData, "webhook")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if state.TriggerType != "webhook" {
		t.Errorf("TriggerType = %q, want 'webhook'", state.TriggerType)
	}

	if state.StartTime.IsZero() {
		t.Errorf("StartTime should not be zero")
	}
}

func TestEngine_Execute_SwitchRoutesAllMatchingConditions(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:producer", &testExecutor{output: map[string]any{
		"status_code": 200,
		"response":    map[string]any{"status": "ok"},
	}})
	registry.Register(node.TypeLogicSwitch, &logic.SwitchNode{})
	registry.Register("test:first", &testExecutor{output: map[string]any{"branch": "first"}})
	registry.Register("test:second", &testExecutor{output: map[string]any{"branch": "second"}})
	registry.Register("test:default", &testExecutor{output: map[string]any{"branch": "default"}})

	engine := pipeline.NewEngine(registry)

	switchConfig := json.RawMessage(`{
		"conditions": [
			{"id": "status-ok", "label": "Status OK", "expression": "input.status_code == 200"},
			{"id": "response-ok", "label": "Response OK", "expression": "input.response.status == \"ok\""}
		]
	}`)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "producer", Type: "test:producer"},
			{ID: "switch", Type: string(node.TypeLogicSwitch), Data: switchConfig},
			{ID: "first", Type: "test:first"},
			{ID: "second", Type: "test:second"},
			{ID: "default", Type: "test:default"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "producer", Target: "switch"},
			{ID: "e2", Source: "switch", SourceHandle: "status-ok", Target: "first"},
			{ID: "e3", Source: "switch", SourceHandle: "response-ok", Target: "second"},
			{ID: "e4", Source: "switch", SourceHandle: "default", Target: "default"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	for _, nodeID := range []string{"producer", "switch", "first", "second"} {
		if _, ok := state.NodeResults[nodeID]; !ok {
			t.Fatalf("expected result for node %q", nodeID)
		}
	}

	if _, ok := state.NodeResults["default"]; ok {
		t.Fatalf("did not expect default branch to execute when conditions matched")
	}
}

func TestEngine_Execute_SwitchUsesDefaultWhenNothingMatches(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:producer", &testExecutor{output: map[string]any{
		"status_code": 503,
		"response":    map[string]any{"status": "error"},
	}})
	registry.Register(node.TypeLogicSwitch, &logic.SwitchNode{})
	registry.Register("test:matched", &testExecutor{output: map[string]any{"branch": "matched"}})
	registry.Register("test:default", &testExecutor{output: map[string]any{"branch": "default"}})

	engine := pipeline.NewEngine(registry)

	switchConfig := json.RawMessage(`{
		"conditions": [
			{"id": "status-ok", "label": "Status OK", "expression": "input.status_code == 200"}
		]
	}`)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "producer", Type: "test:producer"},
			{ID: "switch", Type: string(node.TypeLogicSwitch), Data: switchConfig},
			{ID: "matched", Type: "test:matched"},
			{ID: "default", Type: "test:default"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "producer", Target: "switch"},
			{ID: "e2", Source: "switch", SourceHandle: "status-ok", Target: "matched"},
			{ID: "e3", Source: "switch", SourceHandle: "default", Target: "default"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, ok := state.NodeResults["matched"]; ok {
		t.Fatalf("did not expect matched branch to execute")
	}

	if _, ok := state.NodeResults["default"]; !ok {
		t.Fatalf("expected default branch to execute when no conditions matched")
	}
}

func TestEngine_Execute_MergeNodeCombinesUpstreamObjects(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:left", &testExecutor{output: map[string]any{
		"metrics": map[string]any{"cpu": 40},
		"left":    true,
	}})
	registry.Register("test:right", &testExecutor{output: map[string]any{
		"metrics": map[string]any{"memory": 72},
		"right":   true,
	}})
	registry.Register(node.TypeLogicMerge, &logic.MergeNode{})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "left", Type: "test:left"},
			{ID: "right", Type: "test:right"},
			{ID: "merge", Type: string(node.TypeLogicMerge), Data: json.RawMessage(`{"mode":"deep"}`)},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "left", Target: "merge"},
			{ID: "e2", Source: "right", Target: "merge"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result := state.NodeResults["merge"]
	if result == nil {
		t.Fatalf("expected merge result")
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := output["left"]; got != true {
		t.Fatalf("left = %#v, want true", got)
	}
	if got := output["right"]; got != true {
		t.Fatalf("right = %#v, want true", got)
	}

	metrics, ok := output["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics has unexpected type %T", output["metrics"])
	}
	if got := metrics["cpu"]; got != float64(40) {
		t.Fatalf("metrics.cpu = %#v, want 40", got)
	}
	if got := metrics["memory"]; got != float64(72) {
		t.Fatalf("metrics.memory = %#v, want 72", got)
	}
}

func TestEngine_Execute_AggregateNodeCollectsBranchOutputs(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:left", &testExecutor{output: map[string]any{"name": "left"}})
	registry.Register("test:right", &testExecutor{output: map[string]any{"name": "right"}})
	registry.Register(node.TypeLogicAggregate, &logic.AggregateNode{})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{
				ID:   "left",
				Type: "test:left",
				Data: json.RawMessage(`{"label":"Left Source"}`),
			},
			{
				ID:   "right",
				Type: "test:right",
				Data: json.RawMessage(`{"label":"Right Source"}`),
			},
			{ID: "aggregate", Type: string(node.TypeLogicAggregate)},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "left", Target: "aggregate"},
			{ID: "e2", Source: "right", Target: "aggregate"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result := state.NodeResults["aggregate"]
	if result == nil {
		t.Fatalf("expected aggregate result")
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if got := output["count"]; got != float64(2) {
		t.Fatalf("count = %#v, want 2", got)
	}

	items, ok := output["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v, want 2 entries", output["items"])
	}

	entries, ok := output["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("entries = %#v, want 2 entries", output["entries"])
	}

	firstEntry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("first entry has unexpected type %T", entries[0])
	}
	if got := firstEntry["label"]; got != "Left Source" {
		t.Fatalf("first label = %#v, want Left Source", got)
	}
}

func TestEngine_Execute_AggregateNodeOverridesSourceIDsInOutput(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("test:left", &testExecutor{output: map[string]any{"name": "left"}})
	registry.Register("test:right", &testExecutor{output: map[string]any{"name": "right"}})
	registry.Register(node.TypeLogicAggregate, &logic.AggregateNode{})

	engine := pipeline.NewEngine(registry)

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{
				ID:   "left",
				Type: "test:left",
				Data: json.RawMessage(`{"label":"Left Source"}`),
			},
			{
				ID:   "right",
				Type: "test:right",
				Data: json.RawMessage(`{"label":"Right Source"}`),
			},
			{
				ID:   "aggregate",
				Type: string(node.TypeLogicAggregate),
				Data: json.RawMessage(`{"idOverrides":{"left":"primary","right":"secondary"}}`),
			},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "e1", Source: "left", Target: "aggregate"},
			{ID: "e2", Source: "right", Target: "aggregate"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	result := state.NodeResults["aggregate"]
	if result == nil {
		t.Fatalf("expected aggregate result")
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	byNodeID, ok := output["byNodeId"].(map[string]any)
	if !ok {
		t.Fatalf("byNodeId has unexpected type %T", output["byNodeId"])
	}
	if _, exists := byNodeID["left"]; exists {
		t.Fatalf("expected left key to be replaced, got %#v", byNodeID)
	}
	if got := byNodeID["primary"]; got == nil {
		t.Fatalf("expected primary key in byNodeId, got %#v", byNodeID)
	}
	if got := byNodeID["secondary"]; got == nil {
		t.Fatalf("expected secondary key in byNodeId, got %#v", byNodeID)
	}

	entries, ok := output["entries"].([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("entries = %#v, want 2 entries", output["entries"])
	}

	firstEntry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("first entry has unexpected type %T", entries[0])
	}
	if got := firstEntry["nodeId"]; got != "primary" {
		t.Fatalf("first entry nodeId = %#v, want primary", got)
	}
	if got := firstEntry["originalNodeId"]; got != "left" {
		t.Fatalf("first entry originalNodeId = %#v, want left", got)
	}
}

func BenchmarkEngine_Execute_Linear(b *testing.B) {
	registry := node.NewRegistry()
	for i := 0; i < 10; i++ {
		registry.Register(node.NodeType("test:"+string(rune('a'+i))), &testExecutor{output: map[string]any{"i": i}})
	}

	engine := pipeline.NewEngine(registry)

	nodes := make([]pipeline.FlowNode, 10)
	edges := make([]pipeline.FlowEdge, 9)
	for i := 0; i < 10; i++ {
		nodes[i] = pipeline.FlowNode{ID: string(rune('1' + i)), Type: "test:" + string(rune('a'+i))}
		if i > 0 {
			edges[i-1] = pipeline.FlowEdge{
				ID:     "e" + string(rune('0'+i)),
				Source: string(rune('0' + i)),
				Target: string(rune('1' + i)),
			}
		}
	}

	flowData := pipeline.FlowData{Nodes: nodes, Edges: edges}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Execute(context.Background(), flowData, "manual")
		if err != nil {
			b.Fatalf("Execute() error = %v", err)
		}
	}
}
