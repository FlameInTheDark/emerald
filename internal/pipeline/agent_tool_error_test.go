package pipeline_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/llm"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/node/logic"
	"github.com/FlameInTheDark/emerald/internal/pipeline"
)

type stubLLMProviderStore struct {
	provider *models.LLMProvider
}

func (s *stubLLMProviderStore) GetByID(context.Context, string) (*models.LLMProvider, error) {
	return s.provider, nil
}

func (s *stubLLMProviderStore) GetDefault(context.Context) (*models.LLMProvider, error) {
	return s.provider, nil
}

type flakyToolNode struct {
	targets []string
}

func (n *flakyToolNode) Execute(context.Context, json.RawMessage, map[string]any) (*node.NodeResult, error) {
	return nil, fmt.Errorf("tool nodes should not execute on the main pipeline path")
}

func (n *flakyToolNode) Validate(json.RawMessage) error {
	return nil
}

func (n *flakyToolNode) ToolDefinition(context.Context, node.ToolNodeMetadata, json.RawMessage) (*llm.ToolDefinition, error) {
	return &llm.ToolDefinition{
		Type: "function",
		Function: llm.ToolSpec{
			Name:        "lookup_item",
			Description: "Look up an item by target name.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": map[string]any{
						"type": "string",
					},
				},
				"required": []string{"target"},
			},
		},
	}, nil
}

func (n *flakyToolNode) ExecuteTool(_ context.Context, _ json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	var payload struct {
		Target string `json:"target"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, fmt.Errorf("parse tool args: %w", err)
	}

	n.targets = append(n.targets, payload.Target)
	switch payload.Target {
	case "wrong":
		return nil, fmt.Errorf("item %q not found", payload.Target)
	case "correct":
		return map[string]any{
			"item": map[string]any{
				"name": payload.Target,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected target %q", payload.Target)
	}
}

func TestEngine_LLMAgentRetriesAfterToolError(t *testing.T) {
	t.Parallel()

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}

		switch requestCount {
		case 1:
			if len(payload.Messages) != 2 {
				t.Fatalf("first request message count = %d, want 2", len(payload.Messages))
			}
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup_item","arguments":"{\"target\":\"wrong\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`)
		case 2:
			if len(payload.Messages) != 4 {
				t.Fatalf("second request message count = %d, want 4", len(payload.Messages))
			}
			if payload.Messages[2].Role != "assistant" || len(payload.Messages[2].ToolCalls) != 1 {
				t.Fatalf("second request assistant tool call missing: %+v", payload.Messages[2])
			}
			if payload.Messages[3].Role != "tool" {
				t.Fatalf("second request last message role = %q, want tool", payload.Messages[3].Role)
			}
			if !strings.Contains(payload.Messages[3].Content, `"error":"item \"wrong\" not found"`) {
				t.Fatalf("second request tool message content = %q, want embedded error", payload.Messages[3].Content)
			}
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call-2","type":"function","function":{"name":"lookup_item","arguments":"{\"target\":\"correct\"}"}}]}}],"usage":{"prompt_tokens":11,"completion_tokens":4,"total_tokens":15}}`)
		case 3:
			if len(payload.Messages) != 6 {
				t.Fatalf("third request message count = %d, want 6", len(payload.Messages))
			}
			if payload.Messages[5].Role != "tool" {
				t.Fatalf("third request last message role = %q, want tool", payload.Messages[5].Role)
			}
			if !strings.Contains(payload.Messages[5].Content, `"name":"correct"`) {
				t.Fatalf("third request tool success content = %q, want successful result", payload.Messages[5].Content)
			}
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Recovered after tool error."}}],"usage":{"prompt_tokens":12,"completion_tokens":6,"total_tokens":18}}`)
		default:
			t.Fatalf("unexpected provider request count: %d", requestCount)
		}
	}))
	defer server.Close()

	registry := node.NewRegistry()
	registry.Register(node.TypeLLMAgent, &logic.LLMAgentNode{
		Providers: &stubLLMProviderStore{
			provider: &models.LLMProvider{
				ID:           "provider-1",
				Name:         "Test Provider",
				ProviderType: string(llm.ProviderCustom),
				BaseURL:      &server.URL,
				Model:        "test-model",
			},
		},
	})

	toolNode := &flakyToolNode{}
	registry.Register("tool:test_lookup", toolNode)

	engine := pipeline.NewEngine(registry)
	config, err := json.Marshal(map[string]any{
		"providerId": "provider-1",
		"prompt":     "Find the correct item.",
	})
	if err != nil {
		t.Fatalf("marshal agent config: %v", err)
	}

	flowData := pipeline.FlowData{
		Nodes: []pipeline.FlowNode{
			{ID: "agent", Type: string(node.TypeLLMAgent), Data: config},
			{ID: "lookup", Type: "tool:test_lookup"},
		},
		Edges: []pipeline.FlowEdge{
			{ID: "agent-tool", Source: "agent", Target: "lookup", SourceHandle: "tool"},
		},
	}

	state, err := engine.Execute(context.Background(), flowData, "manual")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if requestCount != 3 {
		t.Fatalf("provider request count = %d, want 3", requestCount)
	}
	if got, want := toolNode.targets, []string{"wrong", "correct"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("tool targets = %#v, want %#v", got, want)
	}

	result := state.NodeResults["agent"]
	if result == nil {
		t.Fatal("expected agent node result")
	}

	var output struct {
		Content     string `json:"content"`
		ToolResults []struct {
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
			Result    map[string]any `json:"result"`
			Error     string         `json:"error"`
		} `json:"toolResults"`
	}
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal agent output: %v", err)
	}

	if got, want := output.Content, "Recovered after tool error."; got != want {
		t.Fatalf("agent content = %q, want %q", got, want)
	}
	if len(output.ToolResults) != 2 {
		t.Fatalf("tool result count = %d, want 2", len(output.ToolResults))
	}
	if got, want := output.ToolResults[0].Error, `item "wrong" not found`; got != want {
		t.Fatalf("first tool error = %q, want %q", got, want)
	}
	if got := output.ToolResults[1].Result["item"]; got == nil {
		t.Fatalf("expected second tool result payload, got %#v", output.ToolResults[1].Result)
	}
}
