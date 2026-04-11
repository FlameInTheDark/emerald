# Plugin Tutorial

This tutorial walks through creating a small Emerald plugin from scratch.

You will build one action node that:

- appears in the normal `Actions` palette
- accepts a URL and an optional bearer token
- performs an HTTP request
- exposes `success` and `error` output pins

At the end, Emerald will load the plugin as a normal node with custom config fields and branching outputs.

## Before You Start

You need:

- a working Emerald checkout
- Go installed
- a place for local plugins, usually `.agents/plugins`

Emerald discovers plugins from:

1. `AUTOMATOR_PLUGINS_DIR`, if set
2. the nearest `.agents/plugins`
3. a fallback `.agents/plugins` under the current working directory

For this tutorial, we will create a self-contained bundle under:

```text
.agents/plugins/hello-http
```

## Step 1: Create the Bundle Directory

Create this layout:

```text
.agents/
  plugins/
    hello-http/
      plugin.json
      go.mod
      main.go
      bin/
```

If you prefer to keep plugin source somewhere else, that also works. The only requirement is that Emerald can find `plugin.json`, and that the manifest points at a valid executable.

## Step 2: Create `plugin.json`

Create `plugin.json` with:

```json
{
  "id": "hello-http",
  "name": "Hello HTTP",
  "version": "0.1.0",
  "description": "Example plugin with one branching HTTP action node.",
  "executable": "./bin/hello-http.exe"
}
```

Notes:

- On Windows, using `.exe` is correct.
- On macOS or Linux, the executable is usually `./bin/hello-http` instead.
- The `id` becomes part of the node type, so keep it stable once people start using the plugin.

Emerald will expose nodes from this plugin under:

```text
action:plugin/hello-http/<node-id>
tool:plugin/hello-http/<node-id>
```

## Step 3: Create `go.mod`

Inside the plugin directory, initialize a Go module:

```powershell
go mod init example.com/hello-http
go get github.com/FlameInTheDark/emerald@latest
```

If you are developing the plugin against a local Emerald checkout and want to pin to your current workspace, you can add a `replace` directive:

```go
module example.com/hello-http

go 1.26.1

require github.com/FlameInTheDark/emerald v0.0.0

replace github.com/FlameInTheDark/emerald => H:/Projects/Go/src/github.com/FlameInTheDark/emerald
```

That keeps your plugin building against the same local SDK code Emerald is using.

## Step 4: Write the Plugin

Create `main.go` with this full example:

```go
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

type fetchConfig struct {
	URL         string `json:"url"`
	Method      string `json:"method"`
	BearerToken string `json:"bearerToken"`
}

type fetchAction struct{}

func (a *fetchAction) ValidateConfig(_ context.Context, config json.RawMessage) error {
	cfg, err := decodeFetchConfig(config)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (a *fetchAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (any, error) {
	cfg, err := decodeFetchConfig(config)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if strings.TrimSpace(cfg.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.BearerToken))
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return map[string]any{
		"status_code": resp.StatusCode,
		"status":      resp.Status,
		"body":        decodeJSONOrString(bodyBytes),
		"source":      "hello-http",
		"matches": map[string]bool{
			"success": resp.StatusCode < http.StatusBadRequest,
			"error":   resp.StatusCode >= http.StatusBadRequest,
		},
	}, nil
}

func main() {
	bundle := &pluginapi.Bundle{
		Info: pluginapi.PluginInfo{
			ID:         "hello-http",
			Name:       "Hello HTTP",
			Version:    "0.1.0",
			APIVersion: pluginapi.APIVersion,
			Nodes: []pluginapi.NodeSpec{
				{
					ID:          "fetch_json",
					Kind:        pluginapi.NodeKindAction,
					Label:       "Fetch JSON",
					Description: "Perform an HTTP request and branch on success or error.",
					Icon:        "globe",
					Color:       "#14b8a6",
					MenuPath:    []string{"Hello HTTP", "Requests"},
					DefaultConfig: map[string]any{
						"url":         "https://api.github.com",
						"method":      "GET",
						"bearerToken": "",
					},
					Fields: []pluginapi.FieldSpec{
						{
							Name:              "url",
							Label:             "URL",
							Type:              pluginapi.FieldTypeString,
							Required:          true,
							Placeholder:       "https://api.example.com/data",
							TemplateSupported: true,
						},
						{
							Name:              "method",
							Label:             "Method",
							Type:              pluginapi.FieldTypeSelect,
							Required:          true,
							Options: []pluginapi.FieldOption{
								{Value: "GET", Label: "GET"},
								{Value: "POST", Label: "POST"},
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
					},
					Outputs: []pluginapi.OutputHandle{
						{ID: "success", Label: "Success", Color: "#22c55e"},
						{ID: "error", Label: "Error", Color: "#ef4444"},
					},
					OutputHints: []pluginapi.OutputHint{
						{Expression: "input.status_code", Label: "HTTP status code"},
						{Expression: "input.status", Label: "HTTP status text"},
						{Expression: "input.body", Label: "Response body"},
					},
				},
			},
		},
		Actions: map[string]pluginapi.ActionNode{
			"fetch_json": &fetchAction{},
		},
	}

	pluginsdk.Serve(bundle)
}

func decodeFetchConfig(raw json.RawMessage) (fetchConfig, error) {
	cfg := fetchConfig{Method: http.MethodGet}
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fetchConfig{}, fmt.Errorf("decode config: %w", err)
	}
	if strings.TrimSpace(cfg.Method) == "" {
		cfg.Method = http.MethodGet
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
```

## Step 5: Understand the Important Parts

The full example is small, but a few pieces matter a lot.

### `pluginapi.Bundle`

`Bundle` is the easiest way to expose a plugin:

- `Info` describes the plugin and its nodes
- `Actions` maps action node IDs to implementations
- `Tools` maps tool node IDs to implementations

If `Info.Nodes` contains an action with ID `fetch_json`, then `Actions` must include `fetch_json` too.

### `NodeSpec`

`NodeSpec` controls how Emerald sees your node:

- `ID` is the stable node ID inside your plugin
- `Kind` is `action` or `tool`
- `Label`, `Description`, `Icon`, and `Color` drive the editor UI
- `MenuPath` controls where the node appears in the palette and context menu
- `DefaultConfig` becomes the initial config for newly created nodes
- `Fields` drives the generic config form
- `Outputs` adds output pins on action nodes
- `OutputHints` improves autocomplete in downstream templated fields

### `ValidateConfig`

`ValidateConfig` receives the raw saved config, not rendered template values.

That means it should validate:

- required keys
- JSON shape
- obviously invalid static values

It should not assume that `{{input.foo}}` or `{{secret.api_token}}` is already resolved.

### `Execute`

`Execute` receives config after Emerald renders templates. If the user enters:

```text
{{secret.api_token}}
```

in the `bearerToken` field, your plugin receives the resolved string value, not the template expression.

The `input` parameter contains the current payload flowing into the node. That is the same runtime object users access in templates as `input`.

### `MenuPath`

`MenuPath` is optional, but it is the easiest way to keep larger plugins organized.

Examples:

- `[]string{"GitHub"}` gives you `Actions -> GitHub -> Fetch JSON`
- `[]string{"GitHub", "Issues"}` gives you `Actions -> GitHub -> Issues -> Fetch JSON`
- `[]string{}` keeps the node at the category root

If you omit `MenuPath` for an action or tool node, Emerald places it under `General`.

### `matches`

Because this node declares custom outputs, the result must include:

```json
{
  "matches": {
    "success": true,
    "error": false
  }
}
```

Every declared output handle must appear in `matches`, and every value must be a boolean.

## Step 6: Build the Binary

From the plugin directory:

```powershell
go build -o .\bin\hello-http.exe .
```

On macOS or Linux:

```bash
go build -o ./bin/hello-http .
```

After this step, your bundle should look like:

```text
.agents/
  plugins/
    hello-http/
      plugin.json
      go.mod
      main.go
      bin/
        hello-http.exe
```

## Step 7: Start or Restart Emerald

Emerald loads plugins when it starts, so restart the server after adding or rebuilding the plugin.

Once Emerald is back up:

- open the editor
- open the `Actions` palette
- search for `Fetch JSON`

You should see your node with the configured globe icon and two output pins: `Success` and `Error`.

## Step 8: Use the Plugin Node in a Pipeline

Create a simple test pipeline:

1. Add your `Fetch JSON` node.
2. Set `URL` to `https://api.github.com`.
3. Leave `Method` as `GET`.
4. Optionally set `Bearer Token` to `{{secret.api_token}}`.
5. Connect the `Success` output to a logging, prompt, or message node.
6. Connect the `Error` output to a different branch.

Downstream nodes can reference output values like:

```text
{{input.status_code}}
{{input.status}}
{{input.body}}
```

Because the action returns a normal JSON object, plugin nodes behave like built-in nodes during templating.

## Step 9: Add a Secret

If your plugin needs credentials:

1. Open `Settings`.
2. Create a secret named `api_token`.
3. Put `{{secret.api_token}}` into a template-enabled field.

At runtime, Emerald resolves the secret before calling your action. The secret value is not returned by normal secret list APIs and is not meant to be stored in ordinary execution context snapshots.

## Step 10: Extend the Plugin with a Tool Node

Once the action node works, you can add a tool node to the same plugin.

Tool nodes are meant to be connected to an `llm:agent` node. They implement:

```go
type ToolNode interface {
	ValidateConfig(ctx context.Context, config json.RawMessage) error
	ToolDefinition(ctx context.Context, meta pluginapi.ToolNodeMetadata, config json.RawMessage) (*pluginapi.ToolDefinition, error)
	ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error)
}
```

The usual pattern is:

1. Add a new `NodeSpec` with `Kind: pluginapi.NodeKindTool`
2. Register a `ToolNode` implementation in `bundle.Tools`
3. Build a JSON-schema-style tool definition in `ToolDefinition`
4. Parse model arguments in `ExecuteTool`
5. Return a JSON-compatible object

Small example:

```go
type echoTool struct{}

func (t *echoTool) ValidateConfig(_ context.Context, config json.RawMessage) error {
	return nil
}

func (t *echoTool) ToolDefinition(_ context.Context, meta pluginapi.ToolNodeMetadata, _ json.RawMessage) (*pluginapi.ToolDefinition, error) {
	return &pluginapi.ToolDefinition{
		Type: "function",
		Function: pluginapi.ToolSpec{
			Name:        "echo_text",
			Description: "Echo text back to the agent.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type": "string",
					},
				},
				"required": []string{"text"},
			},
		},
	}, nil
}

func (t *echoTool) ExecuteTool(_ context.Context, _ json.RawMessage, args json.RawMessage, _ map[string]any) (any, error) {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return nil, err
	}
	return map[string]any{"echo": payload.Text}, nil
}
```

Then register it:

```go
bundle.Info.Nodes = append(bundle.Info.Nodes, pluginapi.NodeSpec{
	ID:          "echo_tool",
	Kind:        pluginapi.NodeKindTool,
	Label:       "Echo Tool",
	Description: "Simple example tool node.",
	Icon:        "wrench",
	Color:       "#38bdf8",
	MenuPath:    []string{"Hello HTTP", "Agent Tools"},
})

bundle.Tools["echo_tool"] = &echoTool{}
```

## Step 11: Know the Main v1 Caveat for Tool Nodes

Tool nodes work, but one runtime detail matters:

- `ToolDefinition` is built before normal pipeline input is available
- `ExecuteTool` runs later with the real pipeline payload

So for v1:

- keep tool-definition config static when possible
- do not depend on `{{input.*}}` while building the tool definition
- put dynamic behavior in `ExecuteTool`

If you need a full working example that includes both an action node and a tool node, use:

[`examples/plugins/sample-request-kit`](/H:/Projects/Go/src/github.com/FlameInTheDark/emerald/examples/plugins/sample-request-kit)

## Step 12: Troubleshoot Faster

If the plugin does not load:

- verify `plugin.json` is in the plugin root tree
- verify the `executable` path is correct
- verify the binary actually exists
- restart Emerald after rebuilding

If the plugin loads but the node is unavailable:

- check whether `PluginInfo.ID` matches the manifest `id`
- check whether `APIVersion` matches `pluginapi.APIVersion`
- check for duplicate node IDs

If the node appears but only one output pin shows up:

- confirm the node declares `Outputs` in `NodeSpec`
- confirm Emerald is loading the latest rebuilt binary
- confirm the editor is seeing the live plugin node definition, not an older stale node instance

## Next Steps

After this tutorial, the most useful follow-ups are:

- read the [plugin reference](./README.md) for field types, manifests, and runtime behavior
- inspect the full sample plugin at [`examples/plugins/sample-request-kit`](/H:/Projects/Go/src/github.com/FlameInTheDark/emerald/examples/plugins/sample-request-kit)
- add a second action node or a real tool node once the first action node is working end to end
