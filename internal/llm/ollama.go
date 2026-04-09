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

type OllamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type ollamaRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Tools    []any          `json:"tools,omitempty"`
	Options  map[string]any `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message struct {
		Role      string     `json:"role"`
		Content   string     `json:"content"`
		ToolCalls []ToolCall `json:"tool_calls"`
	} `json:"message"`
	Done            bool   `json:"done"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	DoneReason      string `json:"done_reason"`
}

func NewOllamaProvider(cfg Config) (*OllamaProvider, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL(ProviderOllama)
	}

	return &OllamaProvider{
		baseURL: baseURL,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
		},
	}, nil
}

func (p *OllamaProvider) Name() string {
	return "Ollama"
}

func (p *OllamaProvider) Type() ProviderType {
	return ProviderOllama
}

func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	apiReq := buildOllamaRequest(req, false)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
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

	var apiResp ollamaResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &ChatResponse{
		Content:   apiResp.Message.Content,
		ToolCalls: apiResp.Message.ToolCalls,
		Usage: Usage{
			PromptTokens:     apiResp.PromptEvalCount,
			CompletionTokens: apiResp.EvalCount,
			TotalTokens:      apiResp.PromptEvalCount + apiResp.EvalCount,
		},
	}, nil
}

func (p *OllamaProvider) ChatStream(ctx context.Context, req ChatRequest, handler StreamHandler) (*ChatResponse, error) {
	apiReq := buildOllamaRequest(req, true)

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
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

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var contentBuilder strings.Builder
	var toolCalls []ToolCall
	var usage Usage

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk ollamaResponse
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}

		if chunk.Message.Content != "" {
			contentBuilder.WriteString(chunk.Message.Content)
			if handler != nil {
				if err := handler(StreamEvent{
					Type:  StreamEventContentDelta,
					Delta: chunk.Message.Content,
				}); err != nil {
					return nil, err
				}
			}
		}

		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = chunk.Message.ToolCalls
		}

		if chunk.PromptEvalCount > 0 || chunk.EvalCount > 0 {
			usage = Usage{
				PromptTokens:     chunk.PromptEvalCount,
				CompletionTokens: chunk.EvalCount,
				TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if handler != nil && (usage.TotalTokens > 0 || usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		if err := handler(StreamEvent{
			Type:  StreamEventUsage,
			Usage: usage,
		}); err != nil {
			return nil, err
		}
	}

	return &ChatResponse{
		Content:   contentBuilder.String(),
		ToolCalls: toolCalls,
		Usage:     usage,
	}, nil
}

func buildOllamaRequest(req ChatRequest, stream bool) ollamaRequest {
	apiReq := ollamaRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   stream,
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
