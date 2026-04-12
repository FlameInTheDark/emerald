package channels

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestSplitDiscordMessageKeepsPartsWithinLimit(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("a", 1990) + "\n" + strings.Repeat("b", 50)
	parts := splitDiscordMessage(text, discordMessageCharLimit)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	for index, part := range parts {
		if got := len([]rune(part)); got > discordMessageCharLimit {
			t.Fatalf("part %d length = %d, exceeds limit %d", index, got, discordMessageCharLimit)
		}
	}
	if parts[0] != strings.Repeat("a", 1990)+"\n" {
		t.Fatalf("unexpected first part ending: %q", parts[0][max(0, len(parts[0])-5):])
	}
	if parts[1] != strings.Repeat("b", 50) {
		t.Fatalf("unexpected second part: %q", parts[1])
	}
}

func TestSendDiscordMessagePartsSplitsAndReturnsAggregatePayload(t *testing.T) {
	t.Parallel()

	text := strings.Repeat("a", 1990) + "\n" + strings.Repeat("b", 50)
	sentMessages := make([]*discordgo.MessageSend, 0, 2)

	result, err := sendDiscordMessageParts("chat-1", text, "https://example.com/connect", func(chatID string, message *discordgo.MessageSend) (*discordgo.Message, error) {
		if chatID != "chat-1" {
			t.Fatalf("unexpected chat id: %s", chatID)
		}
		sentMessages = append(sentMessages, message)
		return &discordgo.Message{
			ID:      fmt.Sprintf("msg-%d", len(sentMessages)),
			Content: message.Content,
		}, nil
	})
	if err != nil {
		t.Fatalf("sendDiscordMessageParts returned error: %v", err)
	}

	if len(sentMessages) != 2 {
		t.Fatalf("expected 2 sent messages, got %d", len(sentMessages))
	}
	if len(sentMessages[0].Components) != 0 {
		t.Fatalf("expected first chunk to have no button components, got %#v", sentMessages[0].Components)
	}
	if len(sentMessages[1].Components) == 0 {
		t.Fatal("expected last chunk to include button components")
	}

	if got, want := result["id"], "msg-2"; got != want {
		t.Fatalf("result.id = %#v, want %#v", got, want)
	}
	if got, want := result["message_id"], "msg-2"; got != want {
		t.Fatalf("result.message_id = %#v, want %#v", got, want)
	}
	if got, want := result["parts_count"], 2; got != want {
		t.Fatalf("result.parts_count = %#v, want %#v", got, want)
	}
	if got, want := result["split"], true; got != want {
		t.Fatalf("result.split = %#v, want %#v", got, want)
	}

	ids, ok := result["message_ids"].([]string)
	if !ok {
		t.Fatalf("result.message_ids has unexpected type %T", result["message_ids"])
	}
	if !reflect.DeepEqual(ids, []string{"msg-1", "msg-2"}) {
		t.Fatalf("result.message_ids = %#v, want %#v", ids, []string{"msg-1", "msg-2"})
	}

	messages, ok := result["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("result.messages has unexpected type %T", result["messages"])
	}
	if len(messages) != 2 {
		t.Fatalf("len(result.messages) = %d, want 2", len(messages))
	}
	if got, want := messages[0]["id"], "msg-1"; got != want {
		t.Fatalf("first message id = %#v, want %#v", got, want)
	}
	if got, want := messages[1]["id"], "msg-2"; got != want {
		t.Fatalf("second message id = %#v, want %#v", got, want)
	}
}
