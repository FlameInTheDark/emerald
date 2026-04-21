package pipeline

import (
	"strings"
	"testing"
)

func TestValidateFlowDataRejectsToolNodeInMainExecutionChain(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "trigger-1", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "tool-1", Data: []byte(`{"type":"tool:http"}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "trigger-1", Target: "tool-1"},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "cannot be part of the main execution chain") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlowDataRejectsReturnNodeOutgoingEdge(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "trigger-1", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "return-1", Data: []byte(`{"type":"logic:return"}`)},
			{ID: "action-1", Data: []byte(`{"type":"action:http"}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-0", Source: "trigger-1", Target: "return-1"},
			{ID: "edge-1", Source: "return-1", Target: "action-1"},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "cannot have outgoing edges") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlowDataAllowsVisualGroupWithoutEdges(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "group-1", Data: []byte(`{"type":"visual:group"}`)},
			{ID: "trigger-1", Type: "trigger:manual"},
		},
	}

	if err := ValidateFlowData(flowData); err != nil {
		t.Fatalf("expected validation to succeed, got %v", err)
	}
}

func TestValidateFlowDataRejectsVisualGroupEdges(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "group-1", Data: []byte(`{"type":"visual:group"}`)},
			{ID: "action-1", Data: []byte(`{"type":"action:http"}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "group-1", Target: "action-1"},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "visual group node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlowDataRejectsUnsupportedContinueOnErrorPolicy(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "trigger-1", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "condition-1", Data: []byte(`{"type":"logic:condition","config":{"expression":"true","errorPolicy":"continue"}}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "trigger-1", Target: "condition-1"},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "errorPolicy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlowDataAllowsContinueOnErrorPolicyForActionNodes(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "trigger-1", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "action-1", Data: []byte(`{"type":"action:http","config":{"url":"https://example.com","errorPolicy":"continue"}}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "trigger-1", Target: "action-1"},
		},
	}

	if err := ValidateFlowData(flowData); err != nil {
		t.Fatalf("expected validation to succeed, got %v", err)
	}
}

func TestValidateFlowDataRejectsAggregateDuplicateResolvedIDs(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "trigger-1", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "left", Data: []byte(`{"type":"action:http"}`)},
			{ID: "right", Data: []byte(`{"type":"action:http"}`)},
			{ID: "aggregate", Data: []byte(`{"type":"logic:aggregate","config":{"idOverrides":{"left":"shared","right":"shared"}}}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "trigger-1", Target: "left"},
			{ID: "edge-2", Source: "trigger-1", Target: "right"},
			{ID: "edge-3", Source: "left", Target: "aggregate"},
			{ID: "edge-4", Source: "right", Target: "aggregate"},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "aggregate output id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlowDataRejectsExecutableRootWithoutTrigger(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "run-other", Data: []byte(`{"type":"action:pipeline_run","config":{"pipelineId":"pipeline-2"}}`)},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "must be a trigger node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFlowDataRejectsMultipleManualTriggerRoots(t *testing.T) {
	t.Parallel()

	flowData := FlowData{
		Nodes: []FlowNode{
			{ID: "trigger-1", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "trigger-2", Data: []byte(`{"type":"trigger:manual"}`)},
			{ID: "action-1", Data: []byte(`{"type":"action:http"}`)},
			{ID: "action-2", Data: []byte(`{"type":"action:http"}`)},
		},
		Edges: []FlowEdge{
			{ID: "edge-1", Source: "trigger-1", Target: "action-1"},
			{ID: "edge-2", Source: "trigger-2", Target: "action-2"},
		},
	}

	err := ValidateFlowData(flowData)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "only one manual trigger root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
