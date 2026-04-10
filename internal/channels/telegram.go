package channels

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"

	dbmodels "github.com/FlameInTheDark/emerald/internal/db/models"
)

type telegramRuntime struct {
	service *Service
	channel dbmodels.Channel
	bot     *tgbot.Bot

	mu    sync.Mutex
	state State
}

func newTelegramRuntime(service *Service, channel dbmodels.Channel, cfg Config) (*telegramRuntime, error) {
	state, err := ParseState(&channel)
	if err != nil {
		return nil, err
	}

	runtime := &telegramRuntime{
		service: service,
		channel: channel,
		state:   state,
	}

	client, err := tgbot.New(
		cfg.BotToken,
		tgbot.WithDefaultHandler(runtime.handleUpdate),
		tgbot.WithAllowedUpdates(tgbot.AllowedUpdates{"message"}),
		tgbot.WithInitialOffset(int64(state.LastTelegramUpdateID+1)),
		tgbot.WithHTTPClient(30*time.Second, service.httpClient),
		tgbot.WithNotAsyncHandlers(),
		tgbot.WithErrorsHandler(func(err error) {
			if err != nil {
				log.Printf("telegram channel %s handler error: %v", channel.Name, err)
			}
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	runtime.bot = client
	return runtime, nil
}

func (r *telegramRuntime) Run(ctx context.Context) error {
	r.bot.Start(ctx)
	return nil
}

func (r *telegramRuntime) SendMessage(ctx context.Context, chatID string, text string, buttonURL string) (map[string]any, error) {
	params := &tgbot.SendMessageParams{
		ChatID: telegramChatID(chatID),
		Text:   text,
	}

	if buttonURL != "" {
		params.ReplyMarkup = &tgmodels.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgmodels.InlineKeyboardButton{
				{
					{
						Text: "Connect to channel",
						URL:  buttonURL,
					},
				},
			},
		}
	}

	message, err := r.bot.SendMessage(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("send telegram message: %w", err)
	}

	result, err := marshalValueToMap(message)
	if err != nil {
		return nil, fmt.Errorf("serialize telegram response: %w", err)
	}

	return result, nil
}

func (r *telegramRuntime) EditMessage(ctx context.Context, chatID string, messageID string, text string) (map[string]any, error) {
	parsedMessageID, err := strconv.Atoi(strings.TrimSpace(messageID))
	if err != nil {
		return nil, fmt.Errorf("parse telegram message id %q: %w", messageID, err)
	}

	message, err := r.bot.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    telegramChatID(chatID),
		MessageID: parsedMessageID,
		Text:      text,
	})
	if err != nil {
		return nil, fmt.Errorf("edit telegram message: %w", err)
	}

	result, err := marshalValueToMap(message)
	if err != nil {
		return nil, fmt.Errorf("serialize telegram response: %w", err)
	}

	return result, nil
}

func (r *telegramRuntime) ReplyMessage(ctx context.Context, chatID string, replyToMessageID string, text string) (map[string]any, error) {
	parsedMessageID, err := strconv.Atoi(strings.TrimSpace(replyToMessageID))
	if err != nil {
		return nil, fmt.Errorf("parse telegram reply message id %q: %w", replyToMessageID, err)
	}

	message, err := r.bot.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: telegramChatID(chatID),
		Text:   text,
		ReplyParameters: &tgmodels.ReplyParameters{
			MessageID: parsedMessageID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("reply to telegram message: %w", err)
	}

	result, err := marshalValueToMap(message)
	if err != nil {
		return nil, fmt.Errorf("serialize telegram response: %w", err)
	}

	return result, nil
}

func (r *telegramRuntime) Close() error {
	return nil
}

func (r *telegramRuntime) handleUpdate(ctx context.Context, _ *tgbot.Bot, update *tgmodels.Update) {
	if update == nil {
		return
	}
	defer r.persistOffset(ctx, update.ID)

	message := normalizeTelegramMessage(update.Message)
	if message == nil {
		return
	}

	if err := r.service.handleIncomingMessage(ctx, &r.channel, *message); err != nil {
		log.Printf("failed to handle telegram message for channel %s: %v", r.channel.Name, err)
	}
}

func (r *telegramRuntime) persistOffset(ctx context.Context, updateID int64) {
	r.mu.Lock()
	if updateID <= int64(r.state.LastTelegramUpdateID) {
		r.mu.Unlock()
		return
	}

	r.state.LastTelegramUpdateID = int(updateID)
	state := r.state
	r.mu.Unlock()

	encodedState, err := MarshalState(state)
	if err != nil {
		log.Printf("failed to marshal telegram channel state for %s: %v", r.channel.Name, err)
		return
	}

	if err := r.service.channelStore.UpdateState(ctx, r.channel.ID, encodedState); err != nil && ctx.Err() == nil {
		log.Printf("failed to persist telegram channel state for %s: %v", r.channel.Name, err)
	}
}

func normalizeTelegramMessage(message *tgmodels.Message) *IncomingMessage {
	if message == nil || message.From == nil || message.From.IsBot {
		return nil
	}
	if message.Chat.Type != tgmodels.ChatTypePrivate {
		return nil
	}

	text := strings.TrimSpace(message.Text)
	if text == "" {
		return nil
	}

	displayName := firstNonEmpty(
		strings.TrimSpace(strings.Join([]string{message.From.FirstName, message.From.LastName}, " ")),
		message.From.Username,
	)

	return &IncomingMessage{
		ExternalUserID: strconv.FormatInt(message.From.ID, 10),
		ExternalChatID: strconv.FormatInt(message.Chat.ID, 10),
		Username:       message.From.Username,
		DisplayName:    displayName,
		Text:           text,
		MessageID:      strconv.Itoa(message.ID),
		ReplyToMessage: telegramReplyToMessageID(message),
		Raw: map[string]any{
			"message_id":          message.ID,
			"chat_id":             message.Chat.ID,
			"text":                message.Text,
			"date":                message.Date,
			"reply_to_message_id": telegramReplyToMessageID(message),
		},
	}
}

func telegramChatID(chatID string) any {
	trimmed := strings.TrimSpace(chatID)
	if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return parsed
	}
	return trimmed
}

func telegramReplyToMessageID(message *tgmodels.Message) string {
	if message == nil || message.ReplyToMessage == nil {
		return ""
	}

	return strconv.Itoa(message.ReplyToMessage.ID)
}
