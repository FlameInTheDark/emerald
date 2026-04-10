package action

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/node"
)

type stubChannelStore struct {
	channel *models.Channel
}

func (s *stubChannelStore) GetByID(context.Context, string) (*models.Channel, error) {
	return s.channel, nil
}

type stubChannelContactStore struct {
	contact *models.ChannelContact
}

func (s *stubChannelContactStore) GetByID(context.Context, string) (*models.ChannelContact, error) {
	return s.contact, nil
}

type stubChannelSender struct {
	channel        *models.Channel
	chatID         string
	text           string
	result         map[string]any
	replyChannel   *models.Channel
	replyChatID    string
	replyMessageID string
	replyText      string
	replyResult    map[string]any
	editChannel    *models.Channel
	editChatID     string
	editMessageID  string
	editText       string
	editResult     map[string]any
}

func (s *stubChannelSender) SendMessage(_ context.Context, channel *models.Channel, chatID string, text string) (map[string]any, error) {
	s.channel = channel
	s.chatID = chatID
	s.text = text
	return s.result, nil
}

func (s *stubChannelSender) ReplyMessage(_ context.Context, channel *models.Channel, chatID string, replyToMessageID string, text string) (map[string]any, error) {
	s.replyChannel = channel
	s.replyChatID = chatID
	s.replyMessageID = replyToMessageID
	s.replyText = text
	if s.replyResult != nil {
		return s.replyResult, nil
	}
	return s.result, nil
}

func (s *stubChannelSender) EditMessage(_ context.Context, channel *models.Channel, chatID string, messageID string, text string) (map[string]any, error) {
	s.editChannel = channel
	s.editChatID = chatID
	s.editMessageID = messageID
	s.editText = text
	if s.editResult != nil {
		return s.editResult, nil
	}
	return s.result, nil
}

type stubChannelWaiter struct {
	channelID     string
	contactID     string
	chatID        string
	sentMessageID string
	timeout       time.Duration
	result        map[string]any
}

func (s *stubChannelWaiter) WaitForReply(_ context.Context, channelID string, contactID string, chatID string, sentMessageID string, timeout time.Duration) (map[string]any, error) {
	s.channelID = channelID
	s.contactID = contactID
	s.chatID = chatID
	s.sentMessageID = sentMessageID
	s.timeout = timeout
	return s.result, nil
}

func TestChannelSendAndWaitActionExecute(t *testing.T) {
	t.Parallel()

	channel := &models.Channel{ID: "channel-1", Name: "Support"}
	contact := &models.ChannelContact{ID: "contact-1", ChannelID: "channel-1", ExternalChatID: "chat-1"}
	sender := &stubChannelSender{result: map[string]any{"message_id": "sent-42"}}
	waiter := &stubChannelWaiter{
		result: map[string]any{
			"text":       "approved",
			"contact_id": "contact-1",
		},
	}

	executor := &ChannelSendAndWaitAction{
		Channels: &stubChannelStore{channel: channel},
		Contacts: &stubChannelContactStore{contact: contact},
		Sender:   sender,
		Waiter:   waiter,
	}

	config, err := json.Marshal(channelSendAndWaitConfig{
		ChannelID:      "channel-1",
		Recipient:      "contact-1",
		Message:        "Need approval?",
		TimeoutSeconds: 45,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := executor.Execute(context.Background(), config, map[string]any{})
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}

	if got, want := sender.chatID, "chat-1"; got != want {
		t.Fatalf("sender chatID = %q, want %q", got, want)
	}
	if got, want := waiter.sentMessageID, "sent-42"; got != want {
		t.Fatalf("waiter sentMessageID = %q, want %q", got, want)
	}
	if got, want := waiter.timeout, 45*time.Second; got != want {
		t.Fatalf("waiter timeout = %s, want %s", got, want)
	}

	var output map[string]any
	if err := json.Unmarshal(result.Output, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	reply, ok := output["reply"].(map[string]any)
	if !ok {
		t.Fatalf("reply has unexpected type %T", output["reply"])
	}
	if got, want := reply["text"], "approved"; got != want {
		t.Fatalf("reply text = %#v, want %#v", got, want)
	}
}

func TestChannelSendAndWaitToolExecuteUsesToolArgs(t *testing.T) {
	t.Parallel()

	channel := &models.Channel{ID: "channel-1", Name: "Support"}
	contact := &models.ChannelContact{ID: "contact-1", ChannelID: "channel-1", ExternalChatID: "chat-1"}
	sender := &stubChannelSender{result: map[string]any{"id": "discord-msg-7"}}
	waiter := &stubChannelWaiter{
		result: map[string]any{
			"text": "hello",
		},
	}

	executor := &ChannelSendAndWaitToolNode{
		Channels: &stubChannelStore{channel: channel},
		Contacts: &stubChannelContactStore{contact: contact},
		Sender:   sender,
		Waiter:   waiter,
	}

	config, err := json.Marshal(channelSendAndWaitConfig{
		ChannelID:      "channel-1",
		TimeoutSeconds: 120,
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	result, err := executor.ExecuteTool(
		context.Background(),
		config,
		json.RawMessage(`{"message":"Question?","recipient":"contact-1","timeoutSeconds":30}`),
		map[string]any{},
	)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	if got, want := sender.text, "Question?"; got != want {
		t.Fatalf("sender text = %q, want %q", got, want)
	}
	if got, want := waiter.timeout, 30*time.Second; got != want {
		t.Fatalf("waiter timeout = %s, want %s", got, want)
	}
	if got, want := waiter.sentMessageID, "discord-msg-7"; got != want {
		t.Fatalf("waiter sentMessageID = %q, want %q", got, want)
	}

	output, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("tool result has unexpected type %T", result)
	}
	reply, ok := output["reply"].(map[string]any)
	if !ok {
		t.Fatalf("reply has unexpected type %T", output["reply"])
	}
	if !reflect.DeepEqual(reply, waiter.result) {
		t.Fatalf("reply = %#v, want %#v", reply, waiter.result)
	}
}

func TestChannelSendAndWaitToolDefinitionRequiresMessageWhenNoDefaultConfigured(t *testing.T) {
	t.Parallel()

	executor := &ChannelSendAndWaitToolNode{}
	definition, err := executor.ToolDefinition(context.Background(), node.ToolNodeMetadata{Label: "Ask User"}, json.RawMessage(`{"channelId":"channel-1"}`))
	if err != nil {
		t.Fatalf("tool definition: %v", err)
	}

	required, ok := definition.Function.Parameters["required"].([]string)
	if ok {
		if len(required) != 1 || required[0] != "message" {
			t.Fatalf("required = %#v, want [message]", required)
		}
		return
	}

	requiredAny, ok := definition.Function.Parameters["required"].([]any)
	if !ok || len(requiredAny) != 1 || requiredAny[0] != "message" {
		t.Fatalf("required = %#v, want [message]", definition.Function.Parameters["required"])
	}
}
