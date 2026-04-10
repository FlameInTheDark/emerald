package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/assistants"
	"github.com/FlameInTheDark/emerald/internal/skills"
)

func TestEditorAssistantBuildModelMessagesIncludesModeAndContext(t *testing.T) {
	t.Parallel()

	handler := &EditorAssistantHandler{
		skillStore: staticSkillReader{
			items: []skills.Summary{
				{Name: "pipeline-graph-rules", Description: "Graph validation guidance."},
				{Name: "templating-guide", Description: "Template interpolation guidance."},
				{Name: "lua-scripting-guide", Description: "Lua node guidance."},
				{Name: "logic-expression-guide", Description: "Logic expression guidance."},
				{Name: "llm-tool-edge-rules", Description: "Agent tool edge guidance."},
				{Name: "node-catalog", Description: "Node reference."},
				{Name: "pipeline-builder", Description: "Pipeline design workflow."},
			},
		},
	}
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
		nil,
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
	if !strings.Contains(messages[0].Content, "pipeline-graph-rules") || !strings.Contains(messages[0].Content, "get_skill") {
		t.Fatalf("expected skill guidance in system prompt, got %+v", messages[0])
	}
	if messages[1].Role != "system" || !strings.Contains(messages[1].Content, "Demo Pipeline") || !strings.Contains(messages[1].Content, "trigger-1") {
		t.Fatalf("expected editor context message, got %+v", messages[1])
	}
	if messages[4].Role != "user" || messages[4].Content != "Explain the flow." {
		t.Fatalf("final user message = %+v, want Explain the flow.", messages[4])
	}
}

func TestEditorAssistantBuildModelMessagesIncludesAttachedLogContext(t *testing.T) {
	t.Parallel()

	handler := &EditorAssistantHandler{}
	profile := assistants.DefaultProfile(assistants.ScopePipelineEditor)

	messages, err := handler.buildModelMessages(
		profile,
		"ask",
		nil,
		"Use the sample log.",
		assistants.PipelineSnapshot{
			Name:  "Demo Pipeline",
			Nodes: []map[string]any{},
			Edges: []map[string]any{},
		},
		assistants.SelectionSnapshot{},
		&assistants.ExecutionLogAttachment{
			ID: "run-1",
			Execution: assistants.ExecutionLogAttachmentRun{
				ID:          "run-1",
				TriggerType: "manual",
				Status:      "completed",
				StartedAt:   "2026-04-09T10:00:00Z",
			},
			Nodes: []assistants.ExecutionLogAttachmentNode{
				{
					NodeID:   "action-1",
					NodeType: "action:http",
					Status:   "completed",
					Input:    map[string]any{"request_id": "abc"},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("buildModelMessages: %v", err)
	}
	if !strings.Contains(messages[1].Content, "attached_log") || !strings.Contains(messages[1].Content, "run-1") {
		t.Fatalf("expected attached log context in system message, got %+v", messages[1])
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
	}, true, staticSkillReader{
		items: []skills.Summary{
			{Name: "lua-scripting-guide", Description: "Lua node guidance."},
		},
		content: map[string]skills.Skill{
			"lua-scripting-guide": {
				Name:        "lua-scripting-guide",
				Description: "Lua node guidance.",
				Path:        ".agents/skills/lua-scripting-guide/SKILL.md",
				Content:     "# Lua",
			},
		},
	})

	if len(executor.GetAllTools()) != 2 {
		t.Fatalf("tool count = %d, want 2", len(executor.GetAllTools()))
	}

	skillResult, err := executor.Execute(t.Context(), "get_skill", json.RawMessage(`{"name":"lua-scripting-guide"}`))
	if err != nil {
		t.Fatalf("get_skill error: %v", err)
	}
	skillPayload := skillResult.(map[string]any)
	if skillPayload["name"] != "lua-scripting-guide" {
		t.Fatalf("skill name = %v, want lua-scripting-guide", skillPayload["name"])
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

type staticSkillReader struct {
	items   []skills.Summary
	content map[string]skills.Skill
}

func (r staticSkillReader) List() []skills.Summary {
	result := make([]skills.Summary, len(r.items))
	copy(result, r.items)
	return result
}

func (r staticSkillReader) SummaryText() string {
	lines := make([]string, 0, len(r.items))
	for _, item := range r.items {
		line := "- " + item.Name
		if item.Description != "" {
			line += ": " + item.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (r staticSkillReader) GetByName(name string) (skills.Skill, bool) {
	if r.content == nil {
		return skills.Skill{}, false
	}
	skill, ok := r.content[strings.ToLower(strings.TrimSpace(name))]
	return skill, ok
}
