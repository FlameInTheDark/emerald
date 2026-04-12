package channels

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	dbmodels "github.com/FlameInTheDark/emerald/internal/db/models"
)

const discordMessageCharLimit = 2000

type discordRuntime struct {
	service *Service
	channel dbmodels.Channel
	session *discordgo.Session

	ctxMu     sync.RWMutex
	ctx       context.Context
	closeOnce sync.Once
}

func newDiscordRuntime(service *Service, channel dbmodels.Channel, cfg Config) (*discordRuntime, error) {
	session, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}

	runtime := &discordRuntime{
		service: service,
		channel: channel,
		session: session,
	}

	session.Client = service.httpClient
	session.ShouldReconnectOnError = true
	session.ShouldRetryOnRateLimit = true
	session.SyncEvents = true
	session.Identify.Intents = discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	session.AddHandler(runtime.handleMessage)

	return runtime, nil
}

func (r *discordRuntime) Run(ctx context.Context) error {
	r.setContext(ctx)

	if err := r.session.Open(); err != nil {
		return fmt.Errorf("open discord session: %w", err)
	}

	<-ctx.Done()
	return nil
}

func (r *discordRuntime) SendMessage(ctx context.Context, chatID string, text string, buttonURL string) (map[string]any, error) {
	return sendDiscordMessageParts(chatID, text, buttonURL, func(chatID string, message *discordgo.MessageSend) (*discordgo.Message, error) {
		return r.session.ChannelMessageSendComplex(chatID, message)
	})
}

func sendDiscordMessageParts(
	chatID string,
	text string,
	buttonURL string,
	send func(chatID string, message *discordgo.MessageSend) (*discordgo.Message, error),
) (map[string]any, error) {
	parts := splitDiscordMessage(text, discordMessageCharLimit)
	payloads := make([]map[string]any, 0, len(parts))
	messageIDs := make([]string, 0, len(parts))

	var lastPayload map[string]any
	for index, part := range parts {
		currentButtonURL := ""
		if index == len(parts)-1 {
			currentButtonURL = buttonURL
		}

		result, err := send(chatID, buildDiscordMessage(part, currentButtonURL))
		if err != nil {
			return nil, fmt.Errorf("send discord message part %d/%d: %w", index+1, len(parts), err)
		}

		payload, err := marshalValueToMap(result)
		if err != nil {
			return nil, fmt.Errorf("serialize discord response: %w", err)
		}

		if messageID := firstNonEmpty(stringValue(payload["message_id"]), stringValue(payload["id"])); messageID != "" {
			messageIDs = append(messageIDs, messageID)
		}

		payloads = append(payloads, payload)
		lastPayload = payload
	}

	if len(payloads) == 0 {
		return nil, nil
	}
	if len(payloads) == 1 {
		return lastPayload, nil
	}

	merged := make(map[string]any, len(lastPayload)+4)
	for key, value := range lastPayload {
		merged[key] = value
	}
	merged["messages"] = payloads
	merged["message_ids"] = messageIDs
	merged["parts_count"] = len(payloads)
	merged["split"] = true
	if _, ok := merged["message_id"]; !ok {
		if messageID := firstNonEmpty(stringValue(merged["id"])); messageID != "" {
			merged["message_id"] = messageID
		}
	}

	return merged, nil
}

func buildDiscordMessage(text string, buttonURL string) *discordgo.MessageSend {
	message := &discordgo.MessageSend{
		Content: text,
	}

	if buttonURL != "" {
		message.Components = []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label: "Connect to channel",
						Style: discordgo.LinkButton,
						URL:   buttonURL,
					},
				},
			},
		}
	}

	return message
}

func splitDiscordMessage(text string, limit int) []string {
	if limit <= 0 {
		limit = discordMessageCharLimit
	}
	if utf8.RuneCountInString(text) <= limit {
		return []string{text}
	}

	runes := []rune(text)
	parts := make([]string, 0, (len(runes)+limit-1)/limit)
	for len(runes) > limit {
		cut := findDiscordSplitIndex(runes, limit)
		if cut <= 0 || cut > len(runes) {
			cut = min(len(runes), limit)
		}
		parts = append(parts, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		parts = append(parts, string(runes))
	}
	if len(parts) == 0 {
		return []string{""}
	}

	return parts
}

func findDiscordSplitIndex(runes []rune, limit int) int {
	if len(runes) <= limit {
		return len(runes)
	}

	segment := string(runes[:limit])
	for _, separator := range []string{"\n\n", "\n", " "} {
		index := strings.LastIndex(segment, separator)
		if index > 0 {
			return utf8.RuneCountInString(segment[:index+len(separator)])
		}
	}

	return limit
}

func stringValue(value any) string {
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

func (r *discordRuntime) EditMessage(ctx context.Context, chatID string, messageID string, text string) (map[string]any, error) {
	result, err := r.session.ChannelMessageEdit(chatID, messageID, text)
	if err != nil {
		return nil, fmt.Errorf("edit discord message: %w", err)
	}

	payload, err := marshalValueToMap(result)
	if err != nil {
		return nil, fmt.Errorf("serialize discord response: %w", err)
	}

	return payload, nil
}

func (r *discordRuntime) ReplyMessage(ctx context.Context, chatID string, replyToMessageID string, text string) (map[string]any, error) {
	failIfNotExists := false
	result, err := r.session.ChannelMessageSendReply(chatID, text, &discordgo.MessageReference{
		MessageID:       replyToMessageID,
		ChannelID:       chatID,
		FailIfNotExists: &failIfNotExists,
	})
	if err != nil {
		return nil, fmt.Errorf("reply to discord message: %w", err)
	}

	payload, err := marshalValueToMap(result)
	if err != nil {
		return nil, fmt.Errorf("serialize discord response: %w", err)
	}

	return payload, nil
}

func (r *discordRuntime) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		closeErr = r.session.Close()
	})
	return closeErr
}

func (r *discordRuntime) handleMessage(_ *discordgo.Session, event *discordgo.MessageCreate) {
	if event == nil || event.Message == nil || event.Author == nil {
		return
	}
	if event.GuildID != "" || event.Author.Bot || strings.TrimSpace(event.Content) == "" {
		return
	}

	displayName := firstNonEmpty(event.Author.DisplayName(), event.Author.Username)
	if err := r.service.handleIncomingMessage(r.context(), &r.channel, IncomingMessage{
		ExternalUserID: event.Author.ID,
		ExternalChatID: event.ChannelID,
		Username:       event.Author.Username,
		DisplayName:    displayName,
		Text:           event.Content,
		MessageID:      event.ID,
		ReplyToMessage: discordReplyToMessageID(event),
		Raw: map[string]any{
			"id":                  event.ID,
			"channel_id":          event.ChannelID,
			"content":             event.Content,
			"author_id":           event.Author.ID,
			"reply_to_message_id": discordReplyToMessageID(event),
		},
	}); err != nil {
		log.Printf("failed to handle discord message for channel %s: %v", r.channel.Name, err)
	}
}

func (r *discordRuntime) setContext(ctx context.Context) {
	r.ctxMu.Lock()
	defer r.ctxMu.Unlock()
	r.ctx = ctx
}

func (r *discordRuntime) context() context.Context {
	r.ctxMu.RLock()
	defer r.ctxMu.RUnlock()

	if r.ctx != nil {
		return r.ctx
	}

	return context.Background()
}

func discordReplyToMessageID(event *discordgo.MessageCreate) string {
	if event == nil || event.MessageReference == nil {
		return ""
	}

	return event.MessageReference.MessageID
}
