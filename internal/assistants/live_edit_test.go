package assistants

import "testing"

func TestValidateAndApplyOperationsMergesPartialNodeUpdates(t *testing.T) {
	t.Parallel()

	snapshot := PipelineSnapshot{
		Nodes: []map[string]any{
			{
				"id":       "node-1",
				"type":     "emerald",
				"position": map[string]any{"x": 12.0, "y": 24.0},
				"data": map[string]any{
					"type":  "action:http",
					"label": "Original",
					"config": map[string]any{
						"url":    "https://example.com",
						"method": "GET",
					},
				},
			},
		},
		Edges: []map[string]any{},
	}

	operations, nextSnapshot, err := ValidateAndApplyOperations(snapshot, []LivePipelineOperation{
		{
			Type: "update_nodes",
			Nodes: []map[string]any{
				{
					"id": "node-1",
					"data": map[string]any{
						"label": "Updated",
						"config": map[string]any{
							"method": "POST",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ValidateAndApplyOperations returned error: %v", err)
	}

	if len(operations) != 1 || len(operations[0].Nodes) != 1 {
		t.Fatalf("normalized operations = %#v, want one updated node", operations)
	}

	node := operations[0].Nodes[0]
	position := node["position"].(map[string]any)
	if position["x"] != 12.0 || position["y"] != 24.0 {
		t.Fatalf("updated node position = %#v, want preserved coordinates", position)
	}

	data := node["data"].(map[string]any)
	if data["label"] != "Updated" {
		t.Fatalf("updated node label = %#v, want Updated", data["label"])
	}

	config := data["config"].(map[string]any)
	if config["url"] != "https://example.com" || config["method"] != "POST" {
		t.Fatalf("updated node config = %#v, want merged config", config)
	}

	if len(nextSnapshot.Nodes) != 1 {
		t.Fatalf("next snapshot nodes = %#v, want one node", nextSnapshot.Nodes)
	}
}

func TestValidateAndApplyOperationsRejectsAddedNodeWithoutPosition(t *testing.T) {
	t.Parallel()

	_, _, err := ValidateAndApplyOperations(PipelineSnapshot{
		Nodes: []map[string]any{},
		Edges: []map[string]any{},
	}, []LivePipelineOperation{
		{
			Type: "add_nodes",
			Nodes: []map[string]any{
				{
					"id":   "node-1",
					"type": "emerald",
					"data": map[string]any{
						"type": "trigger:manual",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("ValidateAndApplyOperations returned nil error, want node position validation failure")
	}
	if err.Error() != "node position is required" {
		t.Fatalf("ValidateAndApplyOperations error = %q, want node position is required", err.Error())
	}
}
