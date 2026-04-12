package action

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
)

type stubPipelineRunner struct {
	pipelineID string
	input      map[string]any
}

func (s *stubPipelineRunner) Run(_ context.Context, pipelineID string, input map[string]any) (*pipeline.RunResult, error) {
	s.pipelineID = pipelineID
	s.input = input

	return &pipeline.RunResult{
		ExecutionID: "exec-1",
		PipelineID:  pipelineID,
		Status:      "completed",
		Returned:    true,
		ReturnValue: input,
	}, nil
}

type stubSecretReturningPipelineRunner struct{}

func (s *stubSecretReturningPipelineRunner) Run(_ context.Context, pipelineID string, input map[string]any) (*pipeline.RunResult, error) {
	return &pipeline.RunResult{
		ExecutionID: "exec-1",
		PipelineID:  pipelineID,
		Status:      "completed",
		Returned:    true,
		ReturnValue: map[string]any{
			"requestId": "req-1",
			"secret": map[string]any{
				"api_token": "Token",
			},
		},
	}, nil
}

type stubPipelineCatalog struct {
	byID map[string]*models.Pipeline
}

func (s *stubPipelineCatalog) List(context.Context) ([]models.Pipeline, error) {
	return nil, nil
}

func (s *stubPipelineCatalog) GetByID(_ context.Context, id string) (*models.Pipeline, error) {
	if pipelineModel, ok := s.byID[id]; ok {
		return pipelineModel, nil
	}

	return nil, nil
}

func TestPipelineRunToolDefinitionIncludesDynamicPipelineID(t *testing.T) {
	t.Parallel()

	executor := &PipelineRunToolNode{}
	config, err := json.Marshal(runPipelineConfig{
		ToolName:             "Run Support Pipeline",
		ToolDescription:      "Run any support pipeline by id.",
		AllowModelPipelineID: true,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	definition, err := executor.ToolDefinition(context.Background(), node.ToolNodeMetadata{Label: "Pipeline Runner"}, config)
	if err != nil {
		t.Fatalf("tool definition: %v", err)
	}

	if got, want := definition.Function.Name, "run_support_pipeline"; got != want {
		t.Fatalf("tool name = %q, want %q", got, want)
	}
	if got, want := definition.Function.Description, "Run any support pipeline by id."; got != want {
		t.Fatalf("tool description = %q, want %q", got, want)
	}

	properties, ok := definition.Function.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool properties missing or wrong type: %#v", definition.Function.Parameters["properties"])
	}
	if _, ok := properties["pipelineId"]; !ok {
		t.Fatalf("pipelineId property missing from tool definition")
	}

	required, ok := definition.Function.Parameters["required"].([]string)
	if ok {
		if len(required) != 1 || required[0] != "pipelineId" {
			t.Fatalf("required fields = %#v, want [pipelineId]", required)
		}
		return
	}

	requiredAny, ok := definition.Function.Parameters["required"].([]any)
	if !ok {
		t.Fatalf("required fields missing or wrong type: %#v", definition.Function.Parameters["required"])
	}
	if len(requiredAny) != 1 || requiredAny[0] != "pipelineId" {
		t.Fatalf("required fields = %#v, want [pipelineId]", requiredAny)
	}
}

func TestPipelineRunToolExecuteUsesProvidedPipelineID(t *testing.T) {
	t.Parallel()

	runner := &stubPipelineRunner{}
	executor := &PipelineRunToolNode{
		Runner: runner,
	}
	config, err := json.Marshal(runPipelineConfig{
		AllowModelPipelineID: true,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := executor.ExecuteTool(context.Background(), config, json.RawMessage(`{"pipelineId":"pipe-123","params":{"status":"ok"}}`), nil)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	if got, want := runner.pipelineID, "pipe-123"; got != want {
		t.Fatalf("runner pipeline id = %q, want %q", got, want)
	}
	if got, want := runner.input, map[string]any{"status": "ok"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runner input = %#v, want %#v", got, want)
	}

	output, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tool result has unexpected type %T", result)
	}
	if got, want := output["pipeline_id"], "pipe-123"; got != want {
		t.Fatalf("tool output pipeline_id = %#v, want %#v", got, want)
	}
}

func TestPipelineRunToolExecuteInjectsConfiguredArguments(t *testing.T) {
	t.Parallel()

	runner := &stubPipelineRunner{}
	executor := &PipelineRunToolNode{Runner: runner}
	config, err := json.Marshal(runPipelineConfig{
		PipelineID: "pipe-123",
		Arguments: []runPipelineToolArgumentConfig{
			{Name: "ticket", Description: "Ticket to inspect", Required: true},
			{Name: "priority"},
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	_, err = executor.ExecuteTool(
		context.Background(),
		config,
		json.RawMessage(`{"ticket":"INC-42","priority":2,"params":{"message":"hello"}}`),
		nil,
	)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	wantInput := map[string]any{
		"message": "hello",
		"arguments": map[string]any{
			"ticket":   "INC-42",
			"priority": float64(2),
		},
	}
	if !reflect.DeepEqual(runner.input, wantInput) {
		t.Fatalf("runner input = %#v, want %#v", runner.input, wantInput)
	}
}

func TestPipelineRunToolExecuteRedactsReturnedSecrets(t *testing.T) {
	t.Parallel()

	executor := &PipelineRunToolNode{Runner: &stubSecretReturningPipelineRunner{}}
	config, err := json.Marshal(runPipelineConfig{
		PipelineID: "pipe-123",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := executor.ExecuteTool(context.Background(), config, json.RawMessage(`{"params":{"requestId":"req-1"}}`), nil)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	output, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tool result has unexpected type %T", result)
	}

	returnValue, ok := output["return_value"].(map[string]any)
	if !ok {
		t.Fatalf("return_value missing or wrong type: %#v", output["return_value"])
	}
	if _, ok := returnValue["secret"]; ok {
		t.Fatalf("expected return_value.secret to be redacted, got %#v", returnValue["secret"])
	}
}

func TestPipelineGetToolExecuteReturnsPipelineData(t *testing.T) {
	t.Parallel()

	executor := &PipelineGetToolNode{
		Pipelines: &stubPipelineCatalog{
			byID: map[string]*models.Pipeline{
				"pipe-123": {
					ID:          "pipe-123",
					Name:        "Support flow",
					Description: nil,
					Status:      "draft",
					Nodes:       `[{"id":"trigger-1","type":"trigger:manual","data":{"label":"Manual Trigger","type":"trigger:manual","config":{},"enabled":true}}]`,
					Edges:       `[]`,
				},
			},
		},
	}
	config, err := json.Marshal(getPipelineConfig{
		PipelineID:        "pipe-123",
		IncludeDefinition: true,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := executor.ExecuteTool(context.Background(), config, nil, nil)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	output, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tool result has unexpected type %T", result)
	}
	pipelineOutput, ok := output["pipeline"].(map[string]any)
	if !ok {
		t.Fatalf("pipeline output missing or wrong type: %#v", output["pipeline"])
	}
	if got, want := pipelineOutput["id"], "pipe-123"; got != want {
		t.Fatalf("pipeline id = %#v, want %#v", got, want)
	}
	if _, ok := pipelineOutput["nodes"]; !ok {
		t.Fatalf("pipeline nodes missing from output: %#v", pipelineOutput)
	}
}
