package templateops

import (
	"context"
	"encoding/json"
	"testing"
)

type recordingDefinitionValidator struct {
	calls []bool
	err   error
}

func (v *recordingDefinitionValidator) ValidateDefinition(_ context.Context, _ string, _ string, allowUnavailablePlugins bool) error {
	v.calls = append(v.calls, allowUnavailablePlugins)
	return v.err
}

func TestServiceCreateAlwaysAllowsUnavailablePluginsDuringTemplateValidation(t *testing.T) {
	t.Parallel()

	validator := &recordingDefinitionValidator{}
	service := NewService(&stubTemplateStore{}, nil, validator)

	_, err := service.Create(context.Background(), CreateTemplateInput{
		Name: "Plugin Template",
		Definition: Definition{
			Nodes: json.RawMessage(`[{"id":"plugin-node","data":{"type":"action:plugin/acme/http"}}]`),
			Edges: json.RawMessage(`[]`),
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if len(validator.calls) != 1 {
		t.Fatalf("expected 1 validator call, got %d", len(validator.calls))
	}
	if !validator.calls[0] {
		t.Fatalf("expected template validation to allow unavailable plugins, got %v", validator.calls[0])
	}
}
