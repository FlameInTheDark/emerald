package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/node"
)

type channelEventContextKey string

const channelEventKey channelEventContextKey = "channel_event"

type ChannelEvent struct {
	ChannelID      string         `json:"channel_id"`
	ChannelName    string         `json:"channel_name"`
	ChannelType    string         `json:"channel_type"`
	ContactID      string         `json:"contact_id"`
	ExternalUserID string         `json:"external_user_id"`
	ExternalChatID string         `json:"external_chat_id"`
	Username       string         `json:"username,omitempty"`
	DisplayName    string         `json:"display_name,omitempty"`
	Text           string         `json:"text"`
	Message        map[string]any `json:"message,omitempty"`
}

func WithChannelEvent(ctx context.Context, event ChannelEvent) context.Context {
	return context.WithValue(ctx, channelEventKey, event)
}

func ChannelEventFromContext(ctx context.Context) (ChannelEvent, bool) {
	if ctx == nil {
		return ChannelEvent{}, false
	}

	event, ok := ctx.Value(channelEventKey).(ChannelEvent)
	return event, ok
}

func IsTriggerType(nodeType node.NodeType) bool {
	switch nodeType {
	case node.TypeTriggerManual, node.TypeTriggerCron, node.TypeTriggerWebhook, node.TypeTriggerChannel:
		return true
	default:
		return false
	}
}

func MatchesExecution(ctx context.Context, nodeType node.NodeType, config json.RawMessage, triggerType string) bool {
	switch triggerType {
	case "manual":
		return nodeType == node.TypeTriggerManual
	case "cron":
		return nodeType == node.TypeTriggerCron
	case "webhook":
		return nodeType == node.TypeTriggerWebhook
	case "channel":
		if nodeType != node.TypeTriggerChannel {
			return false
		}

		event, ok := ChannelEventFromContext(ctx)
		if !ok {
			return false
		}

		var cfg channelTriggerConfig
		if err := json.Unmarshal(config, &cfg); err != nil {
			return false
		}

		if strings.TrimSpace(cfg.ChannelID) == "" {
			return true
		}

		return strings.TrimSpace(cfg.ChannelID) == event.ChannelID
	default:
		return false
	}
}

type ManualTrigger struct{}

func (e *ManualTrigger) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	output := map[string]any{
		"triggered_by": "manual",
		"input":        input,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ManualTrigger) Validate(config json.RawMessage) error {
	return nil
}

type CronTrigger struct{}

type cronConfig struct {
	Schedule string `json:"schedule"`
	Timezone string `json:"timezone"`
}

func (e *CronTrigger) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg cronConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Timezone == "" {
		cfg.Timezone = "UTC"
	}

	output := map[string]any{
		"triggered_by": "cron",
		"schedule":     cfg.Schedule,
		"timezone":     cfg.Timezone,
		"input":        input,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *CronTrigger) Validate(config json.RawMessage) error {
	var cfg cronConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Schedule == "" {
		return fmt.Errorf("schedule is required")
	}
	return nil
}

type WebhookTrigger struct{}

type webhookConfig struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

func (e *WebhookTrigger) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg webhookConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	output := map[string]any{
		"triggered_by": "webhook",
		"path":         cfg.Path,
		"method":       cfg.Method,
		"input":        input,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *WebhookTrigger) Validate(config json.RawMessage) error {
	var cfg webhookConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Path == "" {
		return fmt.Errorf("path is required")
	}
	return nil
}

type ChannelMessageTrigger struct{}

type channelTriggerConfig struct {
	ChannelID string `json:"channelId"`
}

func (e *ChannelMessageTrigger) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg channelTriggerConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	event, ok := ChannelEventFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("channel trigger event is missing")
	}

	if cfg.ChannelID != "" && cfg.ChannelID != event.ChannelID {
		return nil, fmt.Errorf("channel trigger does not match event channel")
	}

	output := map[string]any{
		"triggered_by":     "channel",
		"channel_id":       event.ChannelID,
		"channel_name":     event.ChannelName,
		"channel_type":     event.ChannelType,
		"contact_id":       event.ContactID,
		"external_user_id": event.ExternalUserID,
		"external_chat_id": event.ExternalChatID,
		"chat_id":          event.ExternalChatID,
		"user_id":          event.ExternalUserID,
		"username":         event.Username,
		"display_name":     event.DisplayName,
		"text":             event.Text,
		"message":          event.Message,
		"input":            input,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ChannelMessageTrigger) Validate(config json.RawMessage) error {
	var cfg channelTriggerConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}
