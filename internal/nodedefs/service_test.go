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

func TestBuiltinDefinitionsApplyDefaultMenuPaths(t *testing.T) {
	t.Parallel()

	definitions := BuiltinDefinitions()
	byType := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		byType[definition.Type] = definition
	}

	if got := byType["action:http"].MenuPath; len(got) != 1 || got[0] != "General" {
		t.Fatalf("expected action:http menu path to be [General], got %#v", got)
	}
	if got := byType["action:proxmox_list_nodes"].MenuPath; len(got) != 1 || got[0] != "Proxmox" {
		t.Fatalf("expected proxmox menu path to be [Proxmox], got %#v", got)
	}
	if got := byType["tool:kubernetes_pod_logs"].MenuPath; len(got) != 1 || got[0] != "Kubernetes" {
		t.Fatalf("expected kubernetes menu path to be [Kubernetes], got %#v", got)
	}
}

func TestPluginDefinitionFromBindingUsesConfiguredMenuPath(t *testing.T) {
	t.Parallel()

	definition := pluginDefinitionFromBinding(anyBinding{
		Type:       "action:plugin/acme/request",
		PluginID:   "acme",
		PluginName: "Acme Service",
		Spec: pluginapi.NodeSpec{
			ID:       "request",
			Kind:     pluginapi.NodeKindAction,
			Label:    "Acme Request",
			MenuPath: []string{"Acme Service", "Requests"},
		},
	})

	if len(definition.MenuPath) != 2 || definition.MenuPath[0] != "Acme Service" || definition.MenuPath[1] != "Requests" {
		t.Fatalf("expected configured plugin menu path to be preserved, got %#v", definition.MenuPath)
	}
}
