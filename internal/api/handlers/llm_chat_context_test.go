package handlers

import (
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestBuildStoredMessageReplayStripsReasoningFromContextMessages(t *testing.T) {
	t.Parallel()

	contextMessages := `[{"role":"assistant","reasoning":"private chain of thought","content":"final answer"}]`
	replay, err := buildStoredMessageReplay(models.ChatMessage{
		Role:            "assistant",
		Content:         "final answer",
		ContextMessages: &contextMessages,
	})
	if err != nil {
		t.Fatalf("buildStoredMessageReplay returned error: %v", err)
	}

	if len(replay) != 1 {
		t.Fatalf("expected 1 replay message, got %d", len(replay))
	}
	if replay[0].Reasoning != "" {
		t.Fatalf("expected reasoning to be stripped, got %q", replay[0].Reasoning)
	}
	if replay[0].Content != "final answer" {
		t.Fatalf("unexpected replay content: %q", replay[0].Content)
	}
}

func TestDecodeConversationMessagesExposesContextMessages(t *testing.T) {
	t.Parallel()

	contextMessages := `[{"role":"assistant","reasoning":"inspect state","content":"I checked it."},{"role":"tool","name":"list_nodes","tool_call_id":"call-1","content":"{\"result\":{\"items\":[]}}"}]`
	messages, err := decodeConversationMessages([]models.ChatMessage{
		{
			ID:              "msg-1",
			Role:            "assistant",
			Content:         "I checked it.",
			ContextMessages: &contextMessages,
			CreatedAt:       time.Unix(0, 0).UTC(),
		},
	})
	if err != nil {
		t.Fatalf("decodeConversationMessages returned error: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 response message, got %d", len(messages))
	}
	if len(messages[0].ContextMessages) != 2 {
		t.Fatalf("expected 2 context messages, got %d", len(messages[0].ContextMessages))
	}
	if messages[0].ContextMessages[0].Reasoning != "inspect state" {
		t.Fatalf("unexpected reasoning: %q", messages[0].ContextMessages[0].Reasoning)
	}
}
