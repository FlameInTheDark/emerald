package llm

import (
	"context"
	"fmt"
)

type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderOllama     ProviderType = "ollama"
	ProviderLMStudio   ProviderType = "lmstudio"
	ProviderGemini     ProviderType = "gemini"
	ProviderGroq       ProviderType = "groq"
	ProviderCustom     ProviderType = "custom"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string   `json:"type"`
	Function ToolSpec `json:"function"`
}

type ToolSpec struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ChatRequest struct {
	Model           string           `json:"model"`
	Messages        []Message        `json:"messages"`
	Tools           []ToolDefinition `json:"tools,omitempty"`
	Temperature     float64          `json:"temperature,omitempty"`
	MaxTokens       int              `json:"max_tokens,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
}

type ChatResponse struct {
	Content   string     `json:"content"`
	Reasoning string     `json:"reasoning,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
	Type() ProviderType
}

type Config struct {
	Name         string         `json:"name"`
	ProviderType ProviderType   `json:"provider_type"`
	APIKey       string         `json:"api_key"`
	BaseURL      string         `json:"base_url"`
	Model        string         `json:"model"`
	ExtraConfig  map[string]any `json:"extra_config"`
}

func NewProvider(cfg Config) (Provider, error) {
	switch cfg.ProviderType {
	case ProviderOpenAI:
		return NewOpenAIProvider(cfg)
	case ProviderOpenRouter:
		return NewOpenRouterProvider(cfg)
	case ProviderOllama:
		return NewOllamaProvider(cfg)
	case ProviderLMStudio:
		return NewLMStudioProvider(cfg)
	case ProviderCustom:
		return NewCustomProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.ProviderType)
	}
}

func DefaultBaseURL(providerType ProviderType) string {
	switch providerType {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderOpenRouter:
		return "https://openrouter.ai/api/v1"
	case ProviderOllama:
		return "http://localhost:11434"
	case ProviderLMStudio:
		return "http://localhost:1234/v1"
	default:
		return ""
	}
}
