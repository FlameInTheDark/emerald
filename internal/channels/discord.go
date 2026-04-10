package channels

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	dbmodels "github.com/FlameInTheDark/emerald/internal/db/models"
)

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

	result, err := r.session.ChannelMessageSendComplex(chatID, message)
	if err != nil {
		return nil, fmt.Errorf("send discord message: %w", err)
	}

	payload, err := marshalValueToMap(result)
	if err != nil {
		return nil, fmt.Errorf("serialize discord response: %w", err)
	}

	return payload, nil
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
