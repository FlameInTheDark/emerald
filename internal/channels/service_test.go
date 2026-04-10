package channels

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

func TestServiceWaitForReplyDeliversMatchingReply(t *testing.T) {
	t.Parallel()

	service := &Service{
		waiters: make(map[string][]*pendingReplyWaiter),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	type waitResult struct {
		payload map[string]any
		err     error
	}
	resultCh := make(chan waitResult, 1)

	go func() {
		payload, err := service.WaitForReply(ctx, "channel-1", "contact-1", "chat-1", "sent-1", 0)
		resultCh <- waitResult{payload: payload, err: err}
	}()

	waitForRegisteredWaiter(t, service, "channel-1", "chat-1")

	matched := service.tryDeliverWaiter(
		&models.Channel{ID: "channel-1", Name: "Support", Type: TypeTelegram},
		&models.ChannelContact{ID: "contact-1", ChannelID: "channel-1"},
		IncomingMessage{
			ExternalUserID: "user-1",
			ExternalChatID: "chat-1",
			Username:       "alice",
			DisplayName:    "Alice",
			Text:           "yes",
			MessageID:      "reply-1",
			ReplyToMessage: "sent-1",
			Raw: map[string]any{
				"message_id":          "reply-1",
				"reply_to_message_id": "sent-1",
			},
		},
	)
	if !matched {
		t.Fatalf("expected waiter to match incoming reply")
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("wait for reply: %v", result.err)
		}
		if got, want := result.payload["text"], "yes"; got != want {
			t.Fatalf("reply text = %#v, want %#v", got, want)
		}
		if got, want := result.payload["contact_id"], "contact-1"; got != want {
			t.Fatalf("reply contact_id = %#v, want %#v", got, want)
		}
		if got, want := result.payload["reply_to_message_id"], "sent-1"; got != want {
			t.Fatalf("reply_to_message_id = %#v, want %#v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for waiter result")
	}
}

func TestServiceWaitForReplyTimesOut(t *testing.T) {
	t.Parallel()

	service := &Service{
		waiters: make(map[string][]*pendingReplyWaiter),
	}

	_, err := service.WaitForReply(context.Background(), "channel-1", "", "chat-1", "", 25*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out waiting for reply") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func waitForRegisteredWaiter(t *testing.T, service *Service, channelID string, chatID string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	key := waiterKey(channelID, chatID)

	for time.Now().Before(deadline) {
		service.waitMu.Lock()
		count := len(service.waiters[key])
		service.waitMu.Unlock()
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("waiter %s was not registered in time", key)
}
