package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/FlameInTheDark/automator/internal/assistants"
	"github.com/FlameInTheDark/automator/internal/auth"
	"github.com/FlameInTheDark/automator/internal/db"
	"github.com/FlameInTheDark/automator/internal/db/models"
	"github.com/FlameInTheDark/automator/internal/db/query"
	"github.com/FlameInTheDark/automator/internal/llm"
	"github.com/FlameInTheDark/automator/internal/skills"
)

func TestLLMChatHandlerCreatesConversationAndReusesHistory(t *testing.T) {
	t.Parallel()

	var providerRequests []struct {
		Messages []llm.Message `json:"messages"`
	}
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		providerRequests = append(providerRequests, payload)

		content := "First answer"
		if len(providerRequests) > 1 {
			content = "Follow-up answer"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": content,
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer providerServer.Close()

	handler, _ := newLLMChatHandlerTestDeps(t, providerServer.URL)
	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})

	firstRes := performJSONRequest(t, app, http.MethodPost, "/llm/chat", map[string]any{
		"message":     "Hello world",
		"provider_id": "provider-1",
		"integrations": map[string]any{
			"proxmox":    map[string]any{"enabled": false},
			"kubernetes": map[string]any{"enabled": false},
		},
	})
	if firstRes.StatusCode != fiber.StatusOK {
		t.Fatalf("first chat status = %d, want %d", firstRes.StatusCode, fiber.StatusOK)
	}

	var firstPayload struct {
		ConversationID string               `json:"conversation_id"`
		Content        string               `json:"content"`
		Conversation   conversationResponse `json:"conversation"`
	}
	if err := json.NewDecoder(firstRes.Body).Decode(&firstPayload); err != nil {
		t.Fatalf("decode first chat response: %v", err)
	}
	if firstPayload.ConversationID == "" {
		t.Fatal("expected created conversation ID")
	}
	if firstPayload.Content != "First answer" {
		t.Fatalf("content = %q, want %q", firstPayload.Content, "First answer")
	}
	if firstPayload.Conversation.Title != "Hello world" {
		t.Fatalf("conversation title = %q, want %q", firstPayload.Conversation.Title, "Hello world")
	}

	secondRes := performJSONRequest(t, app, http.MethodPost, "/llm/chat", map[string]any{
		"conversation_id": firstPayload.ConversationID,
		"message":         "What about history?",
	})
	if secondRes.StatusCode != fiber.StatusOK {
		t.Fatalf("second chat status = %d, want %d", secondRes.StatusCode, fiber.StatusOK)
	}

	if len(providerRequests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(providerRequests))
	}
	if len(providerRequests[1].Messages) != 4 {
		t.Fatalf("second provider message count = %d, want 4", len(providerRequests[1].Messages))
	}
	if providerRequests[1].Messages[1].Role != "user" || providerRequests[1].Messages[2].Role != "assistant" {
		t.Fatalf("expected persisted history in second provider request, got %+v", providerRequests[1].Messages)
	}

	detailRes := performJSONRequest(t, app, http.MethodGet, "/llm/conversations/"+firstPayload.ConversationID, nil)
	if detailRes.StatusCode != fiber.StatusOK {
		t.Fatalf("conversation detail status = %d, want %d", detailRes.StatusCode, fiber.StatusOK)
	}

	var detail conversationResponse
	if err := json.NewDecoder(detailRes.Body).Decode(&detail); err != nil {
		t.Fatalf("decode conversation detail: %v", err)
	}
	if len(detail.Messages) != 4 {
		t.Fatalf("conversation message count = %d, want 4", len(detail.Messages))
	}
}

func TestLLMChatHandlerScopesConversationAccessByUser(t *testing.T) {
	t.Parallel()

	handler, chatStore := newLLMChatHandlerTestDeps(t, "")
	conversation := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "Private",
		ProxmoxEnabled:    false,
		KubernetesEnabled: false,
	}
	if err := chatStore.AppendTurn(context.Background(), conversation, true, &models.ChatMessage{Role: "user", Content: "hello"}, &models.ChatMessage{Role: "assistant", Content: "hi"}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-2", Username: "user-two"})
	res := performJSONRequest(t, app, http.MethodGet, "/llm/conversations/"+conversation.ID, nil)
	if res.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusNotFound)
	}
}

func TestLLMChatHandlerUpdateConversationPersistsSettings(t *testing.T) {
	t.Parallel()

	handler, chatStore := newLLMChatHandlerTestDeps(t, "")
	conversation := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "Settings",
		ProxmoxEnabled:    true,
		KubernetesEnabled: false,
	}
	if err := chatStore.AppendTurn(context.Background(), conversation, true, &models.ChatMessage{Role: "user", Content: "hello"}, &models.ChatMessage{Role: "assistant", Content: "hi"}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})
	res := performJSONRequest(t, app, http.MethodPut, "/llm/conversations/"+conversation.ID, map[string]any{
		"provider_id": "provider-1",
		"integrations": map[string]any{
			"proxmox":    map[string]any{"enabled": false},
			"kubernetes": map[string]any{"enabled": true, "cluster_id": "k8s-1"},
		},
	})
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("update status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}

	reloaded, err := chatStore.GetConversationByID(context.Background(), "user-1", conversation.ID)
	if err != nil {
		t.Fatalf("GetConversationByID: %v", err)
	}
	if reloaded == nil {
		t.Fatal("expected reloaded conversation")
	}
	if reloaded.ProviderID == nil || *reloaded.ProviderID != "provider-1" {
		t.Fatalf("provider id = %v, want provider-1", reloaded.ProviderID)
	}
	if reloaded.ProxmoxEnabled {
		t.Fatal("expected proxmox to be disabled")
	}
	if !reloaded.KubernetesEnabled {
		t.Fatal("expected kubernetes to be enabled")
	}
	if reloaded.KubernetesClusterID == nil || *reloaded.KubernetesClusterID != "k8s-1" {
		t.Fatalf("kubernetes cluster id = %v, want k8s-1", reloaded.KubernetesClusterID)
	}
}

func TestLLMChatHandlerReplaysToolTranscriptInFollowUpRequests(t *testing.T) {
	t.Parallel()

	var providerRequests []struct {
		Messages []llm.Message `json:"messages"`
	}
	requestCount := 0
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var payload struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		providerRequests = append(providerRequests, payload)

		var response map[string]any
		switch requestCount {
		case 1:
			response = map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]any{
								{
									"id":   "tool-1",
									"type": "function",
									"function": map[string]any{
										"name":      "list_pipelines",
										"arguments": `{}`,
									},
								},
							},
						},
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     20,
					"completion_tokens": 5,
					"total_tokens":      25,
				},
			}
		case 2:
			response = map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "I checked the pipelines.",
						},
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     24,
					"completion_tokens": 6,
					"total_tokens":      30,
				},
			}
		default:
			response = map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "Follow-up complete.",
						},
					},
				},
				"usage": map[string]any{
					"prompt_tokens":     28,
					"completion_tokens": 7,
					"total_tokens":      35,
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer providerServer.Close()

	handler, _ := newLLMChatHandlerTestDeps(t, providerServer.URL)
	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})

	firstRes := performJSONRequest(t, app, http.MethodPost, "/llm/chat", map[string]any{
		"message":     "Use a tool first",
		"provider_id": "provider-1",
		"integrations": map[string]any{
			"proxmox":    map[string]any{"enabled": false},
			"kubernetes": map[string]any{"enabled": false},
		},
	})
	if firstRes.StatusCode != fiber.StatusOK {
		t.Fatalf("first chat status = %d, want %d", firstRes.StatusCode, fiber.StatusOK)
	}

	var firstPayload struct {
		ConversationID string `json:"conversation_id"`
	}
	if err := json.NewDecoder(firstRes.Body).Decode(&firstPayload); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	secondRes := performJSONRequest(t, app, http.MethodPost, "/llm/chat", map[string]any{
		"conversation_id": firstPayload.ConversationID,
		"message":         "Continue from that tool run",
	})
	if secondRes.StatusCode != fiber.StatusOK {
		t.Fatalf("second chat status = %d, want %d", secondRes.StatusCode, fiber.StatusOK)
	}

	if len(providerRequests) != 3 {
		t.Fatalf("provider request count = %d, want 3", len(providerRequests))
	}

	followUpMessages := providerRequests[2].Messages
	if len(followUpMessages) != 6 {
		t.Fatalf("follow-up provider message count = %d, want 6", len(followUpMessages))
	}
	if followUpMessages[2].Role != "assistant" || len(followUpMessages[2].ToolCalls) != 1 {
		t.Fatalf("expected replayed assistant tool-call message, got %+v", followUpMessages[2])
	}
	if followUpMessages[3].Role != "tool" || followUpMessages[3].ToolCallID != "tool-1" {
		t.Fatalf("expected replayed tool message, got %+v", followUpMessages[3])
	}
	if followUpMessages[4].Role != "assistant" || followUpMessages[4].Content != "I checked the pipelines." {
		t.Fatalf("expected replayed final assistant message, got %+v", followUpMessages[4])
	}
}

func TestLLMChatHandlerCompactsConversationContextWithoutExposingArtifacts(t *testing.T) {
	t.Parallel()

	var providerRequests []struct {
		Messages []llm.Message `json:"messages"`
	}
	requestCount := 0
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var payload struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		providerRequests = append(providerRequests, payload)

		content := "Final answer after compaction."
		if requestCount == 1 {
			content = "- User is investigating pipelines\n- Keep the recent chat focused on the latest request"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role":    "assistant",
						"content": content,
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     60,
				"completion_tokens": 12,
				"total_tokens":      72,
			},
		})
	}))
	defer providerServer.Close()

	handler, chatStore := newLLMChatHandlerTestDepsWithConfig(t, providerServer.URL, `{"context_length":120}`)
	conversation := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "Needs compaction",
		ProxmoxEnabled:    false,
		KubernetesEnabled: false,
	}
	for index := 0; index < 6; index++ {
		userMessage := &models.ChatMessage{Role: "user", Content: strings.Repeat("history ", 12) + string(rune('A'+index))}
		assistantMessage := &models.ChatMessage{Role: "assistant", Content: strings.Repeat("reply ", 12) + string(rune('A'+index))}
		if err := chatStore.AppendTurn(context.Background(), conversation, index == 0, userMessage, assistantMessage); err != nil {
			t.Fatalf("AppendTurn %d: %v", index, err)
		}
	}

	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})
	res := performJSONRequest(t, app, http.MethodPost, "/llm/chat", map[string]any{
		"conversation_id": conversation.ID,
		"message":         "Keep going with the latest state",
	})
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("chat status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}

	if len(providerRequests) != 2 {
		t.Fatalf("provider request count = %d, want 2", len(providerRequests))
	}
	if len(providerRequests[1].Messages) < 3 {
		t.Fatalf("expected compacted request to include summary + recent context, got %+v", providerRequests[1].Messages)
	}
	if providerRequests[1].Messages[1].Role != "system" || !strings.Contains(providerRequests[1].Messages[1].Content, "Hidden conversation memory") {
		t.Fatalf("expected hidden summary system message, got %+v", providerRequests[1].Messages[1])
	}
	if strings.Contains(providerRequests[1].Messages[1].Content, "Final answer after compaction.") {
		t.Fatalf("hidden summary unexpectedly contains live assistant response: %+v", providerRequests[1].Messages[1])
	}

	detailRes := performJSONRequest(t, app, http.MethodGet, "/llm/conversations/"+conversation.ID, nil)
	if detailRes.StatusCode != fiber.StatusOK {
		t.Fatalf("conversation detail status = %d, want %d", detailRes.StatusCode, fiber.StatusOK)
	}

	var detail conversationResponse
	if err := json.NewDecoder(detailRes.Body).Decode(&detail); err != nil {
		t.Fatalf("decode conversation detail: %v", err)
	}
	if len(detail.Messages) != 14 {
		t.Fatalf("conversation message count = %d, want 14", len(detail.Messages))
	}
	if detail.CompactionCount == 0 {
		t.Fatal("expected compaction count to increase")
	}
	if detail.ContextWindow != 120 {
		t.Fatalf("context window = %d, want 120", detail.ContextWindow)
	}
}

func TestLLMChatHandlerDeletesConversation(t *testing.T) {
	t.Parallel()

	handler, chatStore := newLLMChatHandlerTestDeps(t, "")
	conversation := &models.ChatConversation{
		UserID:            "user-1",
		Title:             "Delete me",
		ProxmoxEnabled:    false,
		KubernetesEnabled: false,
	}
	if err := chatStore.AppendTurn(context.Background(), conversation, true, &models.ChatMessage{Role: "user", Content: "hello"}, &models.ChatMessage{Role: "assistant", Content: "hi"}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})
	res := performJSONRequest(t, app, http.MethodDelete, "/llm/conversations/"+conversation.ID, nil)
	if res.StatusCode != fiber.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", res.StatusCode, fiber.StatusNoContent)
	}

	detailRes := performJSONRequest(t, app, http.MethodGet, "/llm/conversations/"+conversation.ID, nil)
	if detailRes.StatusCode != fiber.StatusNotFound {
		t.Fatalf("detail status = %d, want %d", detailRes.StatusCode, fiber.StatusNotFound)
	}
}

func TestLLMChatHandlerReturnsRateLimitStatusFromProvider(t *testing.T) {
	t.Parallel()

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"Too Many Requests"}`, http.StatusTooManyRequests)
	}))
	defer providerServer.Close()

	handler, _ := newLLMChatHandlerTestDeps(t, providerServer.URL)
	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})

	res := performJSONRequest(t, app, http.MethodPost, "/llm/chat", map[string]any{
		"message":     "hello",
		"provider_id": "provider-1",
		"integrations": map[string]any{
			"proxmox":    map[string]any{"enabled": false},
			"kubernetes": map[string]any{"enabled": false},
		},
	})
	if res.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusTooManyRequests)
	}
}

func TestLLMChatHandlerStreamIncludesRateLimitStatusInErrorEvent(t *testing.T) {
	t.Parallel()

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"Too Many Requests"}`, http.StatusTooManyRequests)
	}))
	defer providerServer.Close()

	handler, _ := newLLMChatHandlerTestDeps(t, providerServer.URL)
	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})

	res := performJSONRequest(t, app, http.MethodPost, "/llm/chat/stream", map[string]any{
		"message":     "hello",
		"provider_id": "provider-1",
		"integrations": map[string]any{
			"proxmox":    map[string]any{"enabled": false},
			"kubernetes": map[string]any{"enabled": false},
		},
	})
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("stream status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}

	events := decodeSSEPayloads(t, string(body))
	if len(events) == 0 {
		t.Fatalf("expected at least one event, got body %q", string(body))
	}

	first := events[0]
	if eventType, _ := first["type"].(string); eventType != "error" {
		t.Fatalf("event type = %v, want error", first["type"])
	}
	if status, ok := first["status"].(float64); !ok || int(status) != fiber.StatusTooManyRequests {
		t.Fatalf("error status = %v, want %d", first["status"], fiber.StatusTooManyRequests)
	}
}

func TestLLMChatHandlerStreamsAssistantDeltasAndPersistsConversation(t *testing.T) {
	t.Parallel()

	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		if stream, _ := payload["stream"].(bool); !stream {
			t.Fatalf("expected stream=true request, got %+v", payload)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer providerServer.Close()

	handler, _ := newLLMChatHandlerTestDeps(t, providerServer.URL)
	app := newLLMChatTestApp(handler, auth.Session{UserID: "user-1", Username: "user-one"})

	res := performJSONRequest(t, app, http.MethodPost, "/llm/chat/stream", map[string]any{
		"message":     "Stream hello",
		"provider_id": "provider-1",
		"integrations": map[string]any{
			"proxmox":    map[string]any{"enabled": false},
			"kubernetes": map[string]any{"enabled": false},
		},
	})
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("stream status = %d, want %d", res.StatusCode, fiber.StatusOK)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}

	events := decodeSSEPayloads(t, string(body))
	if len(events) < 3 {
		t.Fatalf("event count = %d, want at least 3 (%s)", len(events), string(body))
	}

	if eventType, _ := events[0]["type"].(string); eventType != "assistant_delta" {
		t.Fatalf("first event type = %v, want assistant_delta", events[0]["type"])
	}
	if delta, _ := events[0]["delta"].(string); delta != "Hello" {
		t.Fatalf("first delta = %q, want %q", delta, "Hello")
	}

	lastEvent := events[len(events)-1]
	if eventType, _ := lastEvent["type"].(string); eventType != "done" {
		t.Fatalf("last event type = %v, want done", lastEvent["type"])
	}

	responsePayload, ok := lastEvent["response"].(map[string]any)
	if !ok {
		t.Fatalf("done event missing response payload: %+v", lastEvent)
	}
	if content, _ := responsePayload["content"].(string); content != "Hello world" {
		t.Fatalf("response content = %q, want %q", content, "Hello world")
	}

	conversationID, _ := responsePayload["conversation_id"].(string)
	if strings.TrimSpace(conversationID) == "" {
		t.Fatal("expected streamed response conversation_id")
	}

	detailRes := performJSONRequest(t, app, http.MethodGet, "/llm/conversations/"+conversationID, nil)
	if detailRes.StatusCode != fiber.StatusOK {
		t.Fatalf("conversation detail status = %d, want %d", detailRes.StatusCode, fiber.StatusOK)
	}

	var detail conversationResponse
	if err := json.NewDecoder(detailRes.Body).Decode(&detail); err != nil {
		t.Fatalf("decode conversation detail: %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("conversation message count = %d, want 2", len(detail.Messages))
	}
	if detail.Messages[1].Content != "Hello world" {
		t.Fatalf("assistant message content = %q, want %q", detail.Messages[1].Content, "Hello world")
	}
}

func TestLLMChatHandlerSystemPromptIncludesWorkspaceSkills(t *testing.T) {
	t.Parallel()

	handler, _ := newLLMChatHandlerTestDeps(t, "")
	handler.skillStore = chatSkillReaderStub{
		skill: skills.Skill{
			Name:        "pipeline-builder",
			Description: "Create valid pipeline definitions.",
			Path:        "/workspace/.agents/skills/pipeline-builder/SKILL.md",
			Content:     "# Pipeline Builder",
		},
	}

	prompt := handler.systemPrompt(
		assistants.DefaultProfile(assistants.ScopeChatWindow),
		nil,
		nil,
		conversationSettings{
			ProxmoxEnabled:    false,
			KubernetesEnabled: false,
		},
	)

	if !strings.Contains(prompt, "Local skills are available") {
		t.Fatalf("expected local skills section in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "pipeline-builder") {
		t.Fatalf("expected pipeline-builder skill in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "get_skill") {
		t.Fatalf("expected get_skill guidance in prompt, got %q", prompt)
	}
}

func newLLMChatHandlerTestDeps(t *testing.T, providerURL string) (*LLMChatHandler, *query.ChatStore) {
	return newLLMChatHandlerTestDepsWithConfig(t, providerURL, "")
}

func newLLMChatHandlerTestDepsWithConfig(t *testing.T, providerURL string, providerConfig string) (*LLMChatHandler, *query.ChatStore) {
	t.Helper()

	database, err := db.New(filepath.Join(t.TempDir(), "automator.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	if _, err := database.Exec(`INSERT INTO users (id, username, password) VALUES (?, ?, ?), (?, ?, ?)`,
		"user-1", "user-one", "secret",
		"user-2", "user-two", "secret",
	); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO llm_providers (id, name, provider_type, api_key, base_url, model, config, is_default) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"provider-1", "Provider", "custom", "secret", nullable(providerURL), "test-model", nullable(providerConfig), 1,
	); err != nil {
		t.Fatalf("seed providers: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO kubernetes_clusters (id, name, source_type, kubeconfig, context_name, default_namespace, server) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"k8s-1", "Primary", "manual", "apiVersion: v1", "default", "default", "https://localhost",
	); err != nil {
		t.Fatalf("seed kubernetes clusters: %v", err)
	}

	providerStore := query.NewLLMProviderStore(database.DB, nil)
	clusterStore := query.NewClusterStore(database.DB, nil)
	kubernetesStore := query.NewKubernetesClusterStore(database.DB, nil)
	pipelineStore := query.NewPipelineStore(database.DB)
	chatStore := query.NewChatStore(database.DB)
	assistantProfileStore := assistants.NewStore(query.NewAppConfigStore(database.DB))

	return NewLLMChatHandler(providerStore, clusterStore, kubernetesStore, pipelineStore, chatStore, nil, nil, nil, nil, assistantProfileStore), chatStore
}

func newLLMChatTestApp(handler *LLMChatHandler, session auth.Session) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(authSessionLocalKey, session)
		return c.Next()
	})
	app.Get("/llm/conversations", handler.ListConversations)
	app.Get("/llm/conversations/:id", handler.GetConversation)
	app.Put("/llm/conversations/:id", handler.UpdateConversation)
	app.Delete("/llm/conversations/:id", handler.DeleteConversation)
	app.Post("/llm/chat/stream", handler.ChatStream)
	app.Post("/llm/chat", handler.Chat)
	return app
}

func performJSONRequest(t *testing.T, app *fiber.App, method string, path string, body any) *http.Response {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return res
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func decodeSSEPayloads(t *testing.T, body string) []map[string]any {
	t.Helper()

	chunks := strings.Split(body, "\n\n")
	events := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(chunk, "data:"))
		if data == "" {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			t.Fatalf("decode SSE payload %q: %v", data, err)
		}
		events = append(events, payload)
	}

	return events
}

type chatSkillReaderStub struct {
	skill skills.Skill
}

func (s chatSkillReaderStub) List() []skills.Summary {
	return []skills.Summary{{
		Name:        s.skill.Name,
		Description: s.skill.Description,
		Path:        s.skill.Path,
	}}
}

func (s chatSkillReaderStub) SummaryText() string {
	return "- " + s.skill.Name + ": " + s.skill.Description
}

func (s chatSkillReaderStub) GetByName(name string) (skills.Skill, bool) {
	if strings.EqualFold(strings.TrimSpace(name), s.skill.Name) {
		return s.skill, true
	}
	return skills.Skill{}, false
}
