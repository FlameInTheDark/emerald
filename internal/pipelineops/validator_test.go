package pipelineops

import (
	"context"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

type recordingDefinitionValidator struct {
	calls []bool
	err   error
}

func (v *recordingDefinitionValidator) ValidateDefinition(_ context.Context, _ string, _ string, allowUnavailablePlugins bool) error {
	v.calls = append(v.calls, allowUnavailablePlugins)
	return v.err
}

func TestServiceCreatePassesDraftPluginAllowanceToValidator(t *testing.T) {
	t.Parallel()

	validator := &recordingDefinitionValidator{}
	service := NewService(&stubStore{}, nil, validator)

	err := service.Create(context.Background(), &models.Pipeline{
		Name:   "Draft Plugin Flow",
		Status: StatusDraft,
		Nodes:  `[{"id":"plugin-node","data":{"type":"action:plugin/acme/http"}}]`,
		Edges:  `[]`,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(validator.calls) != 1 {
		t.Fatalf("expected 1 validator call, got %d", len(validator.calls))
	}
	if !validator.calls[0] {
		t.Fatalf("expected draft create to allow unavailable plugins, got %v", validator.calls[0])
	}
}

func TestServiceUpdatePassesActivePluginRequirementToValidator(t *testing.T) {
	t.Parallel()

	validator := &recordingDefinitionValidator{}
	service := NewService(&stubStore{}, nil, validator)

	err := service.Update(context.Background(), &models.Pipeline{
		ID:     "pipeline-1",
		Name:   "Active Plugin Flow",
		Status: StatusActive,
		Nodes:  `[{"id":"plugin-node","data":{"type":"action:plugin/acme/http"}}]`,
		Edges:  `[]`,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(validator.calls) != 1 {
		t.Fatalf("expected 1 validator call, got %d", len(validator.calls))
	}
	if validator.calls[0] {
		t.Fatalf("expected active update to require available plugins, got %v", validator.calls[0])
	}
}
