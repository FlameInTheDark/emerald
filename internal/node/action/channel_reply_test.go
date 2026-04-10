package action

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestChannelReplyActionExecuteUsesIncomingMessage(t *testing.T) {
	t.Parallel()

	channel := &models.Channel{ID: "channel-1", Name: "Support"}
	contact := &models.ChannelContact{ID: "contact-1", ChannelID: "channel-1", ExternalChatID: "chat-1"}
	sender := &stubChannelSender{replyResult: map[string]any{"message_id": "reply-99"}}

	executor := &ChannelReplyAction{
		Channels: &stubChannelStore{channel: channel},
		Contacts: &stubChannelContactStore{contact: contact},
		Sender:   sender,
	}

	config, err := json.Marshal(channelReplyConfig{
		ChannelID: "channel-1",
		Message:   "Thanks for the update",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	input := map[string]any{
		"contact_id": "contact-1",
		"message": map[string]any{
			"message_id": "incoming-42",
		},
	}

	result, err := executor.Execute(context.Background(), config, input)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}

	if got, want := sender.replyChatID, "chat-1"; got != want {
		t.Fatalf("reply chatID = %q, want %q", got, want)
	}
	if got, want := sender.replyMessageID, "incoming-42"; got != want {
		t.Fatalf("reply messageID = %q, want %q", got, want)
	}
	if got, want := sender.replyText, "Thanks for the update"; got != want {
		t.Fatalf("reply text = %q, want %q", got, want)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got, want := output["status"], "replied"; got != want {
		t.Fatalf("status = %#v, want %#v", got, want)
	}
	if got, want := output["reply_to_message_id"], "incoming-42"; got != want {
		t.Fatalf("reply_to_message_id = %#v, want %#v", got, want)
	}
	if got, want := output["message_id"], "reply-99"; got != want {
		t.Fatalf("message_id = %#v, want %#v", got, want)
	}
}
