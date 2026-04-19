package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlameInTheDark/emerald/internal/webtools"
)

func (r *ToolRegistry) registerWebTools() {
	if r.webToolsConfig == nil || r.webToolsClient == nil {
		return
	}

	if r.webToolsConfig.SearchReady {
		r.Register(ToolDefinition{
			Type: "function",
			Function: ToolSpec{
				Name:        "search_web",
				Description: "Search the public web with the configured search provider. Use this to find recent documentation, news, or pages to inspect before opening a specific URL.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "Search query text.",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Optional number of results to return for SearXNG searches. Defaults to 5 and caps at 10.",
						},
					},
					"required": []string{"query"},
				},
			},
		}, func(ctx context.Context, args json.RawMessage) (any, error) {
			var payload struct {
				Query string `json:"query"`
				Limit int    `json:"limit"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return nil, fmt.Errorf("parse tool args: %w", err)
			}

			return r.webToolsClient.Search(ctx, *r.webToolsConfig, webtools.SearchRequest{
				Query: payload.Query,
				Limit: payload.Limit,
			})
		})
	}

	if r.webToolsConfig.PageReadReady {
		r.Register(ToolDefinition{
			Type: "function",
			Function: ToolSpec{
				Name:        "open_web_page",
				Description: "Fetch the contents of a web page. Use the configured default reader or override the mode to use direct HTTP fetching or Jina page reading.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "HTTP or HTTPS URL to open.",
						},
						"mode": map[string]any{
							"type":        "string",
							"description": "Optional override for how the page is read: http or jina. Omit it to use the configured default.",
							"enum":        []string{"http", "jina"},
						},
						"max_characters": map[string]any{
							"type":        "integer",
							"description": "Optional maximum number of characters to return. Defaults to 12000 and caps at 20000.",
						},
					},
					"required": []string{"url"},
				},
			},
		}, func(ctx context.Context, args json.RawMessage) (any, error) {
			var payload struct {
				URL           string `json:"url"`
				Mode          string `json:"mode"`
				MaxCharacters int    `json:"max_characters"`
			}
			if err := json.Unmarshal(args, &payload); err != nil {
				return nil, fmt.Errorf("parse tool args: %w", err)
			}

			mode := webtools.PageObservationMode(strings.ToLower(strings.TrimSpace(payload.Mode)))
			return r.webToolsClient.OpenPage(ctx, *r.webToolsConfig, webtools.OpenPageRequest{
				URL:           payload.URL,
				Mode:          mode,
				MaxCharacters: payload.MaxCharacters,
			})
		})
	}
}
