package pipelineops

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type stubStore struct {
	pipelines []models.Pipeline
}

func (s *stubStore) List(context.Context) ([]models.Pipeline, error) {
	result := make([]models.Pipeline, len(s.pipelines))
	copy(result, s.pipelines)
	return result, nil
}

func (s *stubStore) GetByID(_ context.Context, id string) (*models.Pipeline, error) {
	for _, pipelineModel := range s.pipelines {
		if pipelineModel.ID == id {
			pipelineCopy := pipelineModel
			return &pipelineCopy, nil
		}
	}

	return nil, context.Canceled
}

func (s *stubStore) Create(context.Context, *models.Pipeline) error { return nil }
func (s *stubStore) Update(context.Context, *models.Pipeline) error { return nil }
func (s *stubStore) Delete(context.Context, string) error           { return nil }

func TestNormalizePipelineDefaultsStatusAndEmptyDefinition(t *testing.T) {
	t.Parallel()

	pipelineModel := &models.Pipeline{
		Name: "Test Pipeline",
	}

	if err := NormalizePipeline(pipelineModel); err != nil {
		t.Fatalf("NormalizePipeline returned error: %v", err)
	}

	if got, want := pipelineModel.Status, StatusDraft; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := pipelineModel.Nodes, "[]"; got != want {
		t.Fatalf("nodes = %q, want %q", got, want)
	}
	if got, want := pipelineModel.Edges, "[]"; got != want {
		t.Fatalf("edges = %q, want %q", got, want)
	}
}

func TestValidateDefinitionRejectsMultipleReturnNodes(t *testing.T) {
	t.Parallel()

	nodes := `[{"id":"return-1","data":{"type":"logic:return"}},{"id":"return-2","data":{"type":"logic:return"}}]`
	err := ValidateDefinition(nodes, "[]")
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "only one Return node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDefinitionRejectsToolNodeInMainExecutionChain(t *testing.T) {
	t.Parallel()

	nodes := `[
		{"id":"trigger-1","data":{"type":"trigger:manual"}},
		{"id":"tool-1","data":{"type":"tool:http"}}
	]`
	edges := `[{"id":"edge-1","source":"trigger-1","target":"tool-1"}]`

	err := ValidateDefinition(nodes, edges)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "cannot be part of the main execution chain") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDefinitionRejectsToolHandleToNonToolTarget(t *testing.T) {
	t.Parallel()

	nodes := `[
		{"id":"agent-1","data":{"type":"llm:agent"}},
		{"id":"action-1","data":{"type":"action:http"}}
	]`
	edges := `[{"id":"edge-1","source":"agent-1","sourceHandle":"tool","target":"action-1"}]`

	err := ValidateDefinition(nodes, edges)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "instead of a tool node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDefinitionAllowsVisualGroupWithoutEdges(t *testing.T) {
	t.Parallel()

	nodes := `[
		{"id":"group-1","data":{"type":"visual:group"}},
		{"id":"action-1","data":{"type":"action:http"}}
	]`

	if err := ValidateDefinition(nodes, "[]"); err != nil {
		t.Fatalf("expected validation to succeed, got %v", err)
	}
}

func TestValidateDefinitionRejectsVisualGroupEdges(t *testing.T) {
	t.Parallel()

	nodes := `[
		{"id":"group-1","data":{"type":"visual:group"}},
		{"id":"action-1","data":{"type":"action:http"}}
	]`
	edges := `[{"id":"edge-1","source":"action-1","target":"group-1"}]`

	err := ValidateDefinition(nodes, edges)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "visual group node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDefinitionRejectsUnsupportedContinueOnErrorPolicy(t *testing.T) {
	t.Parallel()

	nodes := `[{"id":"condition-1","data":{"type":"logic:condition","config":{"expression":"true","errorPolicy":"continue"}}}]`

	err := ValidateDefinition(nodes, "[]")
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "errorPolicy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDefinitionAllowsContinueOnErrorPolicyForActionNodes(t *testing.T) {
	t.Parallel()

	nodes := `[{"id":"action-1","data":{"type":"action:http","config":{"url":"https://example.com","errorPolicy":"continue"}}}]`

	if err := ValidateDefinition(nodes, "[]"); err != nil {
		t.Fatalf("expected validation to succeed, got %v", err)
	}
}

func TestValidateDefinitionRejectsAggregateDuplicateResolvedIDs(t *testing.T) {
	t.Parallel()

	nodes := `[
		{"id":"left","data":{"type":"action:http"}},
		{"id":"right","data":{"type":"action:http"}},
		{"id":"aggregate","data":{"type":"logic:aggregate","config":{"idOverrides":{"left":"shared","right":"shared"}}}}
	]`
	edges := `[
		{"id":"edge-1","source":"left","target":"aggregate"},
		{"id":"edge-2","source":"right","target":"aggregate"}
	]`

	err := ValidateDefinition(nodes, edges)
	if err == nil {
		t.Fatal("expected validation to fail")
	}
	if !strings.Contains(err.Error(), "aggregate output id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceResolveByNameRequiresUniqueMatch(t *testing.T) {
	t.Parallel()

	service := NewService(&stubStore{
		pipelines: []models.Pipeline{
			{ID: "pipe-1", Name: "Support"},
			{ID: "pipe-2", Name: "support"},
		},
	}, nil)

	_, err := service.Resolve(context.Background(), Reference{Name: "support"})
	if err == nil {
		t.Fatal("expected duplicate name resolution to fail")
	}
	if !strings.Contains(err.Error(), "multiple pipelines named") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildPipelineOutputIncludesDefinition(t *testing.T) {
	t.Parallel()

	viewport := `{"x":12,"y":24,"zoom":1.2}`
	output, err := BuildPipelineOutput(models.Pipeline{
		ID:          "pipe-1",
		Name:        "Sample",
		Description: nil,
		Nodes:       `[{"id":"trigger-1","data":{"type":"trigger:manual"}}]`,
		Edges:       `[{"id":"edge-1","source":"trigger-1","target":"return-1"}]`,
		Viewport:    &viewport,
		Status:      StatusActive,
		CreatedAt:   time.Unix(100, 0),
		UpdatedAt:   time.Unix(200, 0),
	}, true)
	if err != nil {
		t.Fatalf("BuildPipelineOutput returned error: %v", err)
	}

	if _, ok := output["nodes"].([]any); !ok {
		t.Fatalf("nodes missing or wrong type: %#v", output["nodes"])
	}
	if _, ok := output["edges"].([]any); !ok {
		t.Fatalf("edges missing or wrong type: %#v", output["edges"])
	}
	if _, ok := output["viewport"].(map[string]any); !ok {
		t.Fatalf("viewport missing or wrong type: %#v", output["viewport"])
	}
}
