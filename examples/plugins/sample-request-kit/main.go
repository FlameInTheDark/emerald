package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/pkg/pluginapi"
	"github.com/FlameInTheDark/emerald/pkg/pluginsdk"
)

type branchRequestConfig struct {
	URL         string `json:"url"`
	Method      string `json:"method"`
	BearerToken string `json:"bearerToken"`
	Body        string `json:"body"`
}

type requestToolConfig struct {
	BaseURL     string `json:"baseUrl"`
	BearerToken string `json:"bearerToken"`
}

type requestToolArgs struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

type branchRequestAction struct{}

func (a *branchRequestAction) ValidateConfig(_ context.Context, config json.RawMessage) error {
	cfg, err := decodeBranchRequestConfig(config)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (a *branchRequestAction) Execute(ctx context.Context, config json.RawMessage, _ map[string]any) (any, error) {
	cfg, err := decodeBranchRequestConfig(config)
	if err != nil {
		return nil, err
	}

	response, err := executeRequest(ctx, strings.TrimSpace(cfg.URL), cfg.Method, cfg.BearerToken, cfg.Body)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"status_code": response.StatusCode,
		"status":      response.Status,
		"body":        response.Body,
		"headers":     response.Headers,
		"matches": map[string]bool{
			"success": response.StatusCode < http.StatusBadRequest,
			"error":   response.StatusCode >= http.StatusBadRequest,
		},
	}, nil
}

type requestTool struct{}

func (t *requestTool) ValidateConfig(_ context.Context, config json.RawMessage) error {
	cfg, err := decodeRequestToolConfig(config)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("baseUrl is required")
	}
	return nil
}

func (t *requestTool) ToolDefinition(_ context.Context, meta pluginapi.ToolNodeMetadata, _ json.RawMessage) (*pluginapi.ToolDefinition, error) {
	name := strings.ReplaceAll(strings.TrimSpace(meta.NodeID), "-", "_")
	if name == "" {
		name = "sample_request_tool"
	}

	return &pluginapi.ToolDefinition{
		Type: "function",
		Function: pluginapi.ToolSpec{
			Name:        name,
			Description: "Perform an HTTP request using the configured base URL and authorization settings.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Request path to append to the configured base URL.",
					},
					"method": map[string]any{
						"type":        "string",
						"description": "HTTP method to use. Defaults to GET.",
					},
				},
				"required": []string{"path"},
			},
		},
	}, nil
}

func (t *requestTool) ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	cfg, err := decodeRequestToolConfig(config)
	if err != nil {
		return nil, err
	}

	var parsedArgs requestToolArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &parsedArgs); err != nil {
			return nil, fmt.Errorf("decode tool args: %w", err)
		}
	}
	if strings.TrimSpace(parsedArgs.Path) == "" {
		return nil, fmt.Errorf("path is required")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	path := "/" + strings.TrimLeft(strings.TrimSpace(parsedArgs.Path), "/")

	response, err := executeRequest(ctx, baseURL+path, parsedArgs.Method, cfg.BearerToken, "")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"status_code": response.StatusCode,
		"status":      response.Status,
		"body":        response.Body,
		"headers":     response.Headers,
	}, nil
}

func main() {
	bundle := &pluginapi.Bundle{
		Info: pluginapi.PluginInfo{
			ID:         "sample-request-kit",
			Name:       "Sample Request Kit",
			Version:    "0.1.0",
			APIVersion: pluginapi.APIVersion,
			Nodes: []pluginapi.NodeSpec{
				{
					ID:          "branch_request",
					Kind:        pluginapi.NodeKindAction,
					Label:       "Branch Request",
					Description: "Make an HTTP request and branch on success or error status codes.",
					Icon:        "globe",
					Color:       "#f97316",
					MenuPath:    []string{"Sample Request Kit", "HTTP"},
					DefaultConfig: map[string]any{
						"url":         "",
						"method":      "GET",
						"bearerToken": "{{secret.api_token}}",
						"body":        "",
					},
					Fields: []pluginapi.FieldSpec{
						{
							Name:              "url",
							Label:             "URL",
							Type:              pluginapi.FieldTypeString,
							Required:          true,
							Placeholder:       "https://api.example.com/status",
							TemplateSupported: true,
						},
						{
							Name:              "method",
							Label:             "Method",
							Type:              pluginapi.FieldTypeSelect,
							Required:          true,
							TemplateSupported: false,
							Options: []pluginapi.FieldOption{
								{Value: "GET", Label: "GET"},
								{Value: "POST", Label: "POST"},
								{Value: "PUT", Label: "PUT"},
								{Value: "DELETE", Label: "DELETE"},
							},
							DefaultStringValue: "GET",
						},
						{
							Name:              "bearerToken",
							Label:             "Bearer Token",
							Type:              pluginapi.FieldTypeString,
							Placeholder:       "{{secret.api_token}}",
							TemplateSupported: true,
						},
						{
							Name:              "body",
							Label:             "Request Body",
							Type:              pluginapi.FieldTypeTextarea,
							Placeholder:       "{\"message\":\"hello\"}",
							TemplateSupported: true,
						},
					},
					Outputs: []pluginapi.OutputHandle{
						{ID: "success", Label: "Success", Color: "#22c55e"},
						{ID: "error", Label: "Error", Color: "#ef4444"},
					},
					OutputHints: []pluginapi.OutputHint{
						{Expression: "input.status_code", Label: "HTTP status code"},
						{Expression: "input.body", Label: "Response body"},
					},
				},
				{
					ID:          "request_tool",
					Kind:        pluginapi.NodeKindTool,
					Label:       "Request Tool",
					Description: "Expose a configurable HTTP client as an agent tool.",
					Icon:        "wrench",
					Color:       "#38bdf8",
					MenuPath:    []string{"Sample Request Kit", "Agent Tools"},
					DefaultConfig: map[string]any{
						"baseUrl":     "https://api.example.com",
						"bearerToken": "{{secret.api_token}}",
					},
					Fields: []pluginapi.FieldSpec{
						{
							Name:              "baseUrl",
							Label:             "Base URL",
							Type:              pluginapi.FieldTypeString,
							Required:          true,
							Placeholder:       "https://api.example.com",
							TemplateSupported: true,
						},
						{
							Name:              "bearerToken",
							Label:             "Bearer Token",
							Type:              pluginapi.FieldTypeString,
							Placeholder:       "{{secret.api_token}}",
							TemplateSupported: true,
						},
					},
				},
			},
		},
		Actions: map[string]pluginapi.ActionNode{
			"branch_request": &branchRequestAction{},
		},
		Tools: map[string]pluginapi.ToolNode{
			"request_tool": &requestTool{},
		},
	}

	pluginsdk.Serve(bundle)
}

type requestResponse struct {
	StatusCode int                 `json:"status_code"`
	Status     string              `json:"status"`
	Body       any                 `json:"body"`
	Headers    map[string][]string `json:"headers"`
}

func executeRequest(ctx context.Context, url string, method string, bearerToken string, body string) (*requestResponse, error) {
	httpMethod := strings.ToUpper(strings.TrimSpace(method))
	if httpMethod == "" {
		httpMethod = http.MethodGet
	}

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, httpMethod, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearerToken))
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return &requestResponse{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       decodeJSONOrString(bodyBytes),
		Headers:    resp.Header,
	}, nil
}

func decodeBranchRequestConfig(raw json.RawMessage) (branchRequestConfig, error) {
	cfg := branchRequestConfig{Method: http.MethodGet}
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return branchRequestConfig{}, fmt.Errorf("decode branch request config: %w", err)
	}
	if strings.TrimSpace(cfg.Method) == "" {
		cfg.Method = http.MethodGet
	}
	return cfg, nil
}

func decodeRequestToolConfig(raw json.RawMessage) (requestToolConfig, error) {
	var cfg requestToolConfig
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return requestToolConfig{}, fmt.Errorf("decode request tool config: %w", err)
	}
	return cfg, nil
}

func decodeJSONOrString(raw []byte) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		return decoded
	}

	return trimmed
}
