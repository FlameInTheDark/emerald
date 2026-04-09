package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

type openAIRequest struct {
	Model         string               `json:"model"`
	Messages      []Message            `json:"messages"`
	Tools         []any                `json:"tools,omitempty"`
	Temperature   float64              `json:"temperature,omitempty"`
	MaxTokens     int                  `json:"max_tokens,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

type openAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content   string                  `json:"content"`
			ToolCalls []openAIStreamToolDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}

type openAIStreamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIToolAccumulator struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

func NewOpenAIProvider(cfg Config) (*OpenAIProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL(ProviderOpenAI)
	}

	return &OpenAIProvider{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

func (p *OpenAIProvider) Name() string {
	return "OpenAI"
}

func (p *OpenAIProvider) Type() ProviderType {
	return ProviderOpenAI
}

func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	apiReq := buildOpenAIRequest(req, false)
	return executeOpenAICompatibleChat(ctx, p.httpClient, p.baseURL+"/chat/completions", "Authorization", p.apiKey, apiReq)
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req ChatRequest, handler StreamHandler) (*ChatResponse, error) {
	apiReq := buildOpenAIRequest(req, true)
	return executeOpenAICompatibleChatStream(ctx, p.httpClient, p.baseURL+"/chat/completions", "Authorization", p.apiKey, apiReq, handler)
}

func buildOpenAIRequest(req ChatRequest, stream bool) openAIRequest {
	apiReq := openAIRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      stream,
	}

	if stream {
		apiReq.StreamOptions = &openAIStreamOptions{IncludeUsage: true}
	}

	if len(req.Tools) > 0 {
		tools := make([]any, len(req.Tools))
		for index, tool := range req.Tools {
			tools[index] = tool
		}
		apiReq.Tools = tools
	}

	return apiReq
}

func executeOpenAICompatibleChat(
	ctx context.Context,
	httpClient *http.Client,
	endpoint string,
	authHeader string,
	apiKey string,
	apiReq openAIRequest,
) (*ChatResponse, error) {
	httpReq, err := newOpenAICompatibleRequest(ctx, endpoint, authHeader, apiKey, apiReq)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := apiResp.Choices[0]
	return &ChatResponse{
		Content:   choice.Message.Content,
		ToolCalls: choice.Message.ToolCalls,
		Usage:     apiResp.Usage,
	}, nil
}

func executeOpenAICompatibleChatStream(
	ctx context.Context,
	httpClient *http.Client,
	endpoint string,
	authHeader string,
	apiKey string,
	apiReq openAIRequest,
	handler StreamHandler,
) (*ChatResponse, error) {
	httpReq, err := newOpenAICompatibleRequest(ctx, endpoint, authHeader, apiKey, apiReq)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read response: %w", readErr)
		}
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var contentBuilder strings.Builder
	var usage Usage
	accumulators := make([]openAIToolAccumulator, 0)

	if err := readSSEStream(resp.Body, func(payload string) error {
		if payload == "[DONE]" {
			return nil
		}

		var chunk openAIStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("parse stream chunk: %w", err)
		}

		if chunk.Usage != nil {
			usage = *chunk.Usage
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				contentBuilder.WriteString(choice.Delta.Content)
				if handler != nil {
					if err := handler(StreamEvent{
						Type:  StreamEventContentDelta,
						Delta: choice.Delta.Content,
					}); err != nil {
						return err
					}
				}
			}

			for _, delta := range choice.Delta.ToolCalls {
				accumulators = applyOpenAIToolDelta(accumulators, delta)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	toolCalls := buildAccumulatedToolCalls(accumulators)
	finalResponse := &ChatResponse{
		Content:   contentBuilder.String(),
		ToolCalls: toolCalls,
		Usage:     usage,
	}

	if handler != nil && (usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		if err := handler(StreamEvent{
			Type:  StreamEventUsage,
			Usage: usage,
		}); err != nil {
			return nil, err
		}
	}

	return finalResponse, nil
}

func newOpenAICompatibleRequest(
	ctx context.Context,
	endpoint string,
	authHeader string,
	apiKey string,
	apiReq openAIRequest,
) (*http.Request, error) {
	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if apiKey != "" {
		httpReq.Header.Set(authHeader, "Bearer "+apiKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	return httpReq, nil
}

func readSSEStream(body io.Reader, handlePayload func(payload string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	dataLines := make([]string, 0, 4)
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}

		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		return handlePayload(payload)
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}

	if err := flush(); err != nil {
		return err
	}

	return nil
}

func applyOpenAIToolDelta(accumulators []openAIToolAccumulator, delta openAIStreamToolDelta) []openAIToolAccumulator {
	index := delta.Index
	for len(accumulators) <= index {
		accumulators = append(accumulators, openAIToolAccumulator{})
	}

	accumulator := accumulators[index]
	if delta.ID != "" {
		accumulator.ID = delta.ID
	}
	if delta.Type != "" {
		accumulator.Type = delta.Type
	}
	if delta.Function.Name != "" {
		accumulator.Name = delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		accumulator.Arguments += delta.Function.Arguments
	}
	accumulators[index] = accumulator
	return accumulators
}

func buildAccumulatedToolCalls(accumulators []openAIToolAccumulator) []ToolCall {
	toolCalls := make([]ToolCall, 0, len(accumulators))
	for index, accumulator := range accumulators {
		if accumulator.Name == "" && accumulator.Arguments == "" && accumulator.ID == "" {
			continue
		}

		id := accumulator.ID
		if strings.TrimSpace(id) == "" {
			id = fmt.Sprintf("tool-call-%d", index+1)
		}

		toolType := accumulator.Type
		if strings.TrimSpace(toolType) == "" {
			toolType = "function"
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:   id,
			Type: toolType,
			Function: ToolFunction{
				Name:      accumulator.Name,
				Arguments: accumulator.Arguments,
			},
		})
	}

	if len(toolCalls) == 0 {
		return nil
	}
	return toolCalls
}
