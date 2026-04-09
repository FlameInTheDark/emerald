package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlameInTheDark/automator/internal/assistants"
)

func TestEditorAssistantBuildModelMessagesIncludesModeAndContext(t *testing.T) {
	t.Parallel()

	handler := &EditorAssistantHandler{}
	profile := assistants.DefaultProfile(assistants.ScopePipelineEditor)

	messages, err := handler.buildModelMessages(
		profile,
		"ask",
		[]editorAssistantHistoryMessage{
			{Role: "user", Content: "What does this do?"},
			{Role: "assistant", Content: "It starts from the manual trigger."},
		},
		"Explain the flow.",
		assistants.PipelineSnapshot{
			Name: "Demo Pipeline",
			Nodes: []map[string]any{
				{
					"id":       "trigger-1",
					"position": map[string]any{"x": 0, "y": 0},
					"data": map[string]any{
						"type":   "trigger:manual",
						"label":  "Manual Trigger",
						"config": map[string]any{},
					},
				},
			},
			Edges: []map[string]any{},
		},
		assistants.SelectionSnapshot{
			SelectedNodeID:  "trigger-1",
			SelectedNodeIDs: []string{"trigger-1"},
		},
	)
	if err != nil {
		t.Fatalf("buildModelMessages: %v", err)
	}
	if len(messages) != 5 {
		t.Fatalf("message count = %d, want 5", len(messages))
	}
	if messages[0].Role != "system" || !strings.Contains(messages[0].Content, "Current mode: ask") {
		t.Fatalf("expected ask mode system prompt, got %+v", messages[0])
	}
	if messages[1].Role != "system" || !strings.Contains(messages[1].Content, "Demo Pipeline") || !strings.Contains(messages[1].Content, "trigger-1") {
		t.Fatalf("expected editor context message, got %+v", messages[1])
	}
	if messages[4].Role != "user" || messages[4].Content != "Explain the flow." {
		t.Fatalf("final user message = %+v, want Explain the flow.", messages[4])
	}
}

func TestEditorAssistantToolExecutorAppliesAndRejectsInvalidOperations(t *testing.T) {
	t.Parallel()

	executor := newEditorAssistantToolExecutor(assistants.PipelineSnapshot{
		Nodes: []map[string]any{
			{
				"id":       "trigger-1",
				"position": map[string]any{"x": 0, "y": 0},
				"data": map[string]any{
					"type":   "trigger:manual",
					"label":  "Manual Trigger",
					"config": map[string]any{},
				},
			},
		},
		Edges: []map[string]any{},
	}, true)

	if len(executor.GetAllTools()) != 1 {
		t.Fatalf("tool count = %d, want 1", len(executor.GetAllTools()))
	}

	validResult, err := executor.Execute(t.Context(), "apply_live_pipeline_edits", json.RawMessage(`{
		"operations": [
			{
				"type": "add_nodes",
				"nodes": [
					{
						"id": "action-1",
						"position": { "x": 180, "y": 0 },
						"data": {
							"type": "action:http",
							"label": "HTTP Request",
							"config": { "url": "https://example.com", "method": "GET", "headers": {}, "body": "" }
						}
					}
				]
			},
			{
				"type": "add_edges",
				"edges": [
					{
						"id": "edge-trigger-action",
						"source": "trigger-1",
						"target": "action-1"
					}
				]
			}
		]
	}`))
	if err != nil {
		t.Fatalf("valid execute error: %v", err)
	}

	payload := validResult.(map[string]any)
	operations, ok := payload["operations"].([]assistants.LivePipelineOperation)
	if !ok || len(operations) != 2 {
		t.Fatalf("operations payload = %+v, want two normalized operations", payload["operations"])
	}

	_, err = executor.Execute(t.Context(), "apply_live_pipeline_edits", json.RawMessage(`{
		"operations": [
			{
				"type": "add_edges",
				"edges": [
					{
						"id": "edge-invalid",
						"source": "trigger-1",
						"target": "missing-node"
					}
				]
			}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "missing-node") {
		t.Fatalf("expected missing node validation error, got %v", err)
	}
}
