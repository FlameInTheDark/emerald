package action

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestChannelEditActionExecuteUsesPreviousSendOutput(t *testing.T) {
	t.Parallel()

	channel := &models.Channel{ID: "channel-1", Name: "Support"}
	contact := &models.ChannelContact{ID: "contact-1", ChannelID: "channel-1", ExternalChatID: "chat-1"}
	sender := &stubChannelSender{editResult: map[string]any{"message_id": "sent-42"}}

	executor := &ChannelEditAction{
		Channels: &stubChannelStore{channel: channel},
		Contacts: &stubChannelContactStore{contact: contact},
		Sender:   sender,
	}

	config, err := json.Marshal(channelEditConfig{
		ChannelID: "channel-1",
		Message:   "Updated message",
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	input := map[string]any{
		"contact_id": "contact-1",
		"response": map[string]any{
			"message_id": "sent-42",
		},
	}

	result, err := executor.Execute(context.Background(), config, input)
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}

	if got, want := sender.editChatID, "chat-1"; got != want {
		t.Fatalf("edit chatID = %q, want %q", got, want)
	}
	if got, want := sender.editMessageID, "sent-42"; got != want {
		t.Fatalf("edit messageID = %q, want %q", got, want)
	}
	if got, want := sender.editText, "Updated message"; got != want {
		t.Fatalf("edit text = %q, want %q", got, want)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got, want := output["status"], "edited"; got != want {
		t.Fatalf("status = %#v, want %#v", got, want)
	}
	if got, want := output["message_id"], "sent-42"; got != want {
		t.Fatalf("message_id = %#v, want %#v", got, want)
	}
}
