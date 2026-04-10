package channels

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/db/models"
)

const (
	TypeTelegram = "telegram"
	TypeDiscord  = "discord"
)

type Config struct {
	BotToken string `json:"botToken"`
}

type State struct {
	LastTelegramUpdateID int `json:"lastTelegramUpdateId,omitempty"`
}

type IncomingMessage struct {
	ExternalUserID string
	ExternalChatID string
	Username       string
	DisplayName    string
	Text           string
	MessageID      string
	ReplyToMessage string
	Raw            map[string]any
}

func ParseConfig(channel *models.Channel) (Config, error) {
	if channel == nil {
		return Config{}, fmt.Errorf("channel is required")
	}

	var cfg Config
	if channel.Config != nil && strings.TrimSpace(*channel.Config) != "" {
		if err := json.Unmarshal([]byte(*channel.Config), &cfg); err != nil {
			return Config{}, fmt.Errorf("parse channel config: %w", err)
		}
	}

	if strings.TrimSpace(cfg.BotToken) == "" {
		return Config{}, fmt.Errorf("channel bot token is required")
	}

	return cfg, nil
}

func ParseState(channel *models.Channel) (State, error) {
	if channel == nil || channel.State == nil || strings.TrimSpace(*channel.State) == "" {
		return State{}, nil
	}

	var state State
	if err := json.Unmarshal([]byte(*channel.State), &state); err != nil {
		return State{}, fmt.Errorf("parse channel state: %w", err)
	}

	return state, nil
}

func MarshalState(state State) (*string, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal channel state: %w", err)
	}

	str := string(data)
	return &str, nil
}
