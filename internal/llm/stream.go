package llm

import "context"

type StreamEventType string

const (
	StreamEventContentDelta StreamEventType = "content_delta"
	StreamEventUsage        StreamEventType = "usage"
)

type StreamEvent struct {
	Type  StreamEventType `json:"type"`
	Delta string          `json:"delta,omitempty"`
	Usage Usage           `json:"usage,omitempty"`
}

type StreamHandler func(StreamEvent) error

type StreamingProvider interface {
	ChatStream(ctx context.Context, req ChatRequest, handler StreamHandler) (*ChatResponse, error)
}

func ChatWithStream(ctx context.Context, provider Provider, req ChatRequest, handler StreamHandler) (*ChatResponse, error) {
	if streamer, ok := provider.(StreamingProvider); ok {
		return streamer.ChatStream(ctx, req, handler)
	}

	resp, err := provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	if handler != nil {
		if resp.Content != "" {
			if err := handler(StreamEvent{
				Type:  StreamEventContentDelta,
				Delta: resp.Content,
			}); err != nil {
				return nil, err
			}
		}
		if resp.Usage.TotalTokens > 0 || resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
			if err := handler(StreamEvent{
				Type:  StreamEventUsage,
				Usage: resp.Usage,
			}); err != nil {
				return nil, err
			}
		}
	}

	return resp, nil
}
