package node_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/trigger"
)

type mockExecutor struct {
	shouldFail bool
	output     map[string]any
}

func (e *mockExecutor) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.shouldFail {
		return nil, context.Canceled
	}
	data, _ := json.Marshal(e.output)
	return &node.NodeResult{Output: data}, nil
}

func (e *mockExecutor) Validate(config json.RawMessage) error {
	return nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	tests := []struct {
		name     string
		nodeType node.NodeType
		executor node.NodeExecutor
	}{
		{
			name:     "register manual trigger",
			nodeType: node.TypeTriggerManual,
			executor: &trigger.ManualTrigger{},
		},
		{
			name:     "register webhook trigger",
			nodeType: node.TypeTriggerWebhook,
			executor: &trigger.WebhookTrigger{},
		},
		{
			name:     "register custom executor",
			nodeType: "custom:test",
			executor: &mockExecutor{output: map[string]any{"result": "ok"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := node.NewRegistry()
			registry.Register(tt.nodeType, tt.executor)

			got, err := registry.Get(tt.nodeType)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", tt.nodeType, err)
			}
			if got == nil {
				t.Errorf("Get(%q) returned nil executor", tt.nodeType)
			}
		})
	}
}

func TestRegistry_GetUnknownType(t *testing.T) {
	registry := node.NewRegistry()

	_, err := registry.Get("unknown:type")
	if err == nil {
		t.Errorf("expected error for unknown node type, got nil")
	}
}

func TestRegistry_ListTypes(t *testing.T) {
	registry := node.NewRegistry()
	registry.Register("type:a", &mockExecutor{})
	registry.Register("type:b", &mockExecutor{})
	registry.Register("type:c", &mockExecutor{})

	types := registry.ListTypes()

	if len(types) != 3 {
		t.Errorf("ListTypes() returned %d types, want 3", len(types))
	}

	typeSet := make(map[node.NodeType]bool)
	for _, tp := range types {
		typeSet[tp] = true
	}

	for _, expected := range []node.NodeType{"type:a", "type:b", "type:c"} {
		if !typeSet[expected] {
			t.Errorf("ListTypes() missing expected type %q", expected)
		}
	}
}

func TestRegistry_Execute(t *testing.T) {
	tests := []struct {
		name       string
		executor   node.NodeExecutor
		input      map[string]any
		wantOutput map[string]any
		wantErr    bool
	}{
		{
			name:       "successful execution",
			executor:   &mockExecutor{output: map[string]any{"status": "ok"}},
			input:      map[string]any{"key": "value"},
			wantOutput: map[string]any{"status": "ok"},
			wantErr:    false,
		},
		{
			name:       "failed execution",
			executor:   &mockExecutor{shouldFail: true},
			input:      map[string]any{},
			wantOutput: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := node.NewRegistry()
			registry.Register("test:node", tt.executor)

			exec, err := registry.Get("test:node")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}

			result, err := exec.Execute(context.Background(), nil, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(result.Output, &got); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}

			for k, v := range tt.wantOutput {
				if got[k] != v {
					t.Errorf("output[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestManualTrigger_Execute(t *testing.T) {
	exec := &trigger.ManualTrigger{}

	result, err := exec.Execute(context.Background(), nil, map[string]any{"test": "value"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if output["triggered_by"] != "manual" {
		t.Errorf("triggered_by = %v, want 'manual'", output["triggered_by"])
	}
}

func TestManualTrigger_Validate(t *testing.T) {
	exec := &trigger.ManualTrigger{}

	if err := exec.Validate(nil); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func BenchmarkRegistry_RegisterGet(b *testing.B) {
	registry := node.NewRegistry()
	exec := &mockExecutor{output: map[string]any{"status": "ok"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.Register(node.NodeType("bench:"+string(rune(i))), exec)
		_, _ = registry.Get(node.NodeType("bench:" + string(rune(i))))
	}
}
