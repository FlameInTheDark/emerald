package llm

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const (
	DefaultContextWindow       = 128000
	DefaultOllamaContextWindow = 32768
)

func ResolveContextWindow(cfg Config) int {
	if override := extraConfigInt(cfg.ExtraConfig, "context_length"); override > 0 {
		return override
	}

	model := strings.ToLower(strings.TrimSpace(cfg.Model))
	switch {
	case model == "":
	case strings.Contains(model, "gpt-3.5"):
		return 16385
	case strings.Contains(model, "claude"):
		return 200000
	case strings.Contains(model, "gemini"):
		return DefaultContextWindow
	case strings.Contains(model, "llama"), strings.Contains(model, "mistral"), strings.Contains(model, "qwen"):
		return DefaultOllamaContextWindow
	default:
		switch cfg.ProviderType {
		case ProviderOllama:
			return DefaultOllamaContextWindow
		default:
			return DefaultContextWindow
		}
	}

	switch cfg.ProviderType {
	case ProviderOllama:
		return DefaultOllamaContextWindow
	default:
		return DefaultContextWindow
	}
}

func EstimateMessagesTokens(messages []Message) int {
	total := 0
	for _, message := range messages {
		total += estimateMessageTokens(message)
	}
	if total == 0 {
		return 0
	}
	return total + 4
}

func estimateMessageTokens(message Message) int {
	total := 4
	total += estimateStringTokens(message.Role)
	total += estimateStringTokens(message.Content)
	total += estimateStringTokens(message.ToolCallID)
	total += estimateStringTokens(message.Name)

	if len(message.ToolCalls) > 0 {
		encoded, err := json.Marshal(message.ToolCalls)
		if err == nil {
			total += estimateStringTokens(string(encoded))
		} else {
			for _, call := range message.ToolCalls {
				total += estimateStringTokens(call.ID)
				total += estimateStringTokens(call.Type)
				total += estimateStringTokens(call.Function.Name)
				total += estimateStringTokens(call.Function.Arguments)
			}
		}
	}

	return total
}

func estimateStringTokens(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}

	runeCount := len([]rune(trimmed))
	return int(math.Ceil(float64(runeCount) / 4))
}

func extraConfigInt(extraConfig map[string]any, key string) int {
	if len(extraConfig) == 0 {
		return 0
	}

	value, ok := extraConfig[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed := strings.TrimSpace(typed)
		if parsed == "" {
			return 0
		}
		var number json.Number = json.Number(parsed)
		intValue, err := number.Int64()
		if err == nil {
			return int(intValue)
		}
	}

	return 0
}

func FormatContextUsage(used int, limit int) string {
	if limit <= 0 {
		return formatCompactTokens(used)
	}
	return fmt.Sprintf("%s / %s", formatCompactTokens(used), formatCompactTokens(limit))
}

func formatCompactTokens(value int) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(value)/1_000_000)
	case value >= 1000:
		return fmt.Sprintf("%.1fk", float64(value)/1000)
	default:
		return fmt.Sprintf("%d", value)
	}
}
