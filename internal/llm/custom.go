package llm

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type CustomProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	authHeader string
}

func NewCustomProvider(cfg Config) (*CustomProvider, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required for custom provider")
	}

	authHeader := "Authorization"
	if cfg.ExtraConfig != nil {
		if header, ok := cfg.ExtraConfig["auth_header"].(string); ok {
			authHeader = header
		}
	}

	return &CustomProvider{
		apiKey:     cfg.APIKey,
		baseURL:    cfg.BaseURL,
		model:      cfg.Model,
		authHeader: authHeader,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (p *CustomProvider) Name() string {
	return "Custom"
}

func (p *CustomProvider) Type() ProviderType {
	return ProviderCustom
}

func (p *CustomProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return executeOpenAICompatibleChat(ctx, p.httpClient, resolveOpenAICompatibleChatEndpoint(p.baseURL, ProviderCustom), p.authHeader, p.apiKey, buildOpenAIRequest(req, ProviderCustom, false))
}

func (p *CustomProvider) ChatStream(ctx context.Context, req ChatRequest, handler StreamHandler) (*ChatResponse, error) {
	return executeOpenAICompatibleChatStream(ctx, p.httpClient, resolveOpenAICompatibleChatEndpoint(p.baseURL, ProviderCustom), p.authHeader, p.apiKey, buildOpenAIRequest(req, ProviderCustom, true), handler)
}
