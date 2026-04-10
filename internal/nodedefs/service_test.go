package nodedefs

import (
	"context"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
)

func TestServiceValidateDefinitionAllowsUnavailablePluginNodesForDrafts(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	err := service.ValidateDefinition(
		context.Background(),
		`[{"id":"plugin-node","data":{"type":"action:plugin/acme/http"}}]`,
		`[]`,
		true,
	)
	if err != nil {
		t.Fatalf("expected draft validation to allow unavailable plugins, got %v", err)
	}
}

func TestServiceValidateDefinitionRejectsUnavailablePluginNodesForActiveFlows(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	err := service.ValidateDefinition(
		context.Background(),
		`[{"id":"plugin-node","data":{"type":"action:plugin/acme/http"}}]`,
		`[]`,
		false,
	)
	if err == nil {
		t.Fatal("expected unavailable plugin node to fail validation")
	}
	if !strings.Contains(err.Error(), "unavailable plugin node type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceValidateDefinitionRejectsMissingHandleForPluginActionOutputs(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	service.builtins["action:plugin/acme/router"] = Definition{
		Type:     "action:plugin/acme/router",
		Category: "action",
		Source:   SourcePlugin,
		Label:    "Router",
		Outputs: []pluginapi.OutputHandle{
			{ID: "success", Label: "Success"},
			{ID: "error", Label: "Error"},
		},
	}

	err := service.ValidateDefinition(
		context.Background(),
		`[
			{"id":"router","data":{"type":"action:plugin/acme/router"}},
			{"id":"next","data":{"type":"action:http"}}
		]`,
		`[{"id":"edge-1","source":"router","target":"next"}]`,
		false,
	)
	if err == nil {
		t.Fatal("expected missing output handle to fail validation")
	}
	if !strings.Contains(err.Error(), "must use one of the declared output handles") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceValidateDefinitionRejectsUnknownPluginOutputHandle(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	service.builtins["action:plugin/acme/router"] = Definition{
		Type:     "action:plugin/acme/router",
		Category: "action",
		Source:   SourcePlugin,
		Label:    "Router",
		Outputs: []pluginapi.OutputHandle{
			{ID: "success", Label: "Success"},
			{ID: "error", Label: "Error"},
		},
	}

	err := service.ValidateDefinition(
		context.Background(),
		`[
			{"id":"router","data":{"type":"action:plugin/acme/router"}},
			{"id":"next","data":{"type":"action:http"}}
		]`,
		`[{"id":"edge-1","source":"router","sourceHandle":"missing","target":"next"}]`,
		false,
	)
	if err == nil {
		t.Fatal("expected unknown output handle to fail validation")
	}
	if !strings.Contains(err.Error(), "unknown output handle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceValidateDefinitionAllowsDeclaredPluginOutputHandle(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	service.builtins["action:plugin/acme/router"] = Definition{
		Type:     "action:plugin/acme/router",
		Category: "action",
		Source:   SourcePlugin,
		Label:    "Router",
		Outputs: []pluginapi.OutputHandle{
			{ID: "success", Label: "Success"},
			{ID: "error", Label: "Error"},
		},
	}

	err := service.ValidateDefinition(
		context.Background(),
		`[
			{"id":"router","data":{"type":"action:plugin/acme/router"}},
			{"id":"next","data":{"type":"action:http"}}
		]`,
		`[{"id":"edge-1","source":"router","sourceHandle":"success","target":"next"}]`,
		false,
	)
	if err != nil {
		t.Fatalf("expected declared output handle to validate, got %v", err)
	}
}
