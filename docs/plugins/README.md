# Plugin Development

Emerald supports custom action, trigger, and tool nodes through a Go-first sidecar plugin runtime built on HashiCorp `go-plugin` with gRPC transport.

This section is split into two parts:

- [Plugin Tutorial](./tutorial.md) - a step-by-step walkthrough for building and loading your first plugin.
- This page - the reference for manifests, SDK types, node behavior, and runtime caveats.

## What Plugins Can Add Today

- Custom action nodes under `action:plugin/<plugin-id>/<node-id>`
- Custom trigger nodes under `trigger:plugin/<plugin-id>/<node-id>`
- Custom tool nodes under `tool:plugin/<plugin-id>/<node-id>`
- Custom config fields rendered in the node settings panel
- Custom output handles for action nodes
- Template autocomplete hints for plugin outputs

Current v1 limits:

- No plugin-defined logic nodes
- No custom input pins
- No per-plugin secret ACLs
- Plugins are local admin-installed bundles, not marketplace packages

## Import Paths

The repository directory and Go module now use the `emerald` name:

```go
module github.com/FlameInTheDark/emerald
```

Plugin code should import:

```go
github.com/FlameInTheDark/emerald/pkg/pluginapi
github.com/FlameInTheDark/emerald/pkg/pluginsdk
```

## Discovery Rules

Emerald resolves the plugin root directory in this order:

1. `EMERALD_PLUGINS_DIR`, when set
2. The nearest parent directory that contains `.agents/plugins`
3. A fallback `.agents/plugins` under the current working directory

Emerald walks that root recursively and loads every `plugin.json` file it finds.

Typical layout:

```text
.agents/
  plugins/
    sample-request-kit/
      plugin.json
      bin/
        sample-request-kit.exe
```

Notes:

- Relative `executable` paths are resolved relative to the directory that contains `plugin.json`.
- Hidden directories under the plugin root are skipped while discovering manifests.
- Plugins are loaded when Emerald starts. After changing a plugin manifest or binary, restart Emerald or use `Settings -> Rediscover Plugins`.

## `plugin.json`

Every plugin bundle needs a `plugin.json` manifest.

Example:

```json
{
  "id": "sample-request-kit",
  "name": "Sample Request Kit",
  "version": "0.1.0",
  "description": "Reference plugin bundle with one branching action node and one tool node.",
  "executable": "./bin/sample-request-kit.exe"
}
```

Supported manifest fields:

| Field | Required | Purpose |
| --- | --- | --- |
| `id` | yes | Stable plugin ID used in node types. |
| `name` | yes | Human-readable plugin name shown in the UI. |
| `version` | no | Plugin version shown in status views. |
| `description` | no | Human-readable description. |
| `executable` | yes | Relative or absolute path to the plugin process. |
| `args` | no | Extra command-line arguments for the executable. |
| `env` | no | Extra environment variables passed to the plugin process. |

Runtime checks:

- The manifest `id` must match the runtime plugin ID reported by `Describe`.
- The runtime `APIVersion` must match `pluginapi.APIVersion`.
- Node IDs must be unique inside one plugin.
- Resolved node types must be unique across all loaded plugins.

## Node Placement In Menus

Each plugin node can optionally declare a menu path:

```go
MenuPath: []string{"Service Name", "Requests"},
```

That path controls where the node appears in both the node palette and the editor context menu.

Examples:

- `[]string{"Service Name"}` becomes `Actions -> Service Name -> Node`
- `[]string{"Service Name", "Requests"}` becomes `Actions -> Service Name -> Requests -> Node`
- `[]string{}` keeps the node at the category root

If you omit `MenuPath` entirely for an `action` or `tool` node, Emerald places it under the default `General` group.

## High-Level Runtime Flow

At startup, Emerald:

1. Finds every manifest
2. Starts one long-lived sidecar process per plugin
3. Performs the `go-plugin` handshake
4. Calls `Describe`
5. Registers every declared node in the node-definition service

If a plugin fails to load, its nodes become unavailable and pipelines using them cannot run until the plugin resolves again.

For trigger plugins, Emerald also keeps one long-lived runtime stream per plugin and pushes full active subscription snapshots through it.

## Main SDK Types

The main packages are:

- `pkg/pluginapi` - shared interfaces and data types
- `pkg/pluginsdk` - `go-plugin` and gRPC helpers

The most important types are:

- `pluginapi.Plugin`
- `pluginapi.Bundle`
- `pluginapi.NodeSpec`
- `pluginapi.TriggerNode`
- `pluginapi.TriggerRuntime`
- `pluginapi.FieldSpec`
- `pluginapi.OutputHandle`
- `pluginapi.OutputHint`

## Minimal Plugin Skeleton

```go
package main

import (
  "github.com/FlameInTheDark/emerald/pkg/pluginapi"
  "github.com/FlameInTheDark/emerald/pkg/pluginsdk"
)

func main() {
  bundle := &pluginapi.Bundle{
    Info: pluginapi.PluginInfo{
      ID:         "my-plugin",
      Name:       "My Plugin",
      Version:    "0.1.0",
      APIVersion: pluginapi.APIVersion,
      Nodes: []pluginapi.NodeSpec{
        {
          ID:          "my_action",
          Kind:        pluginapi.NodeKindAction,
          Label:       "My Action",
          Description: "Do something custom.",
          Icon:        "globe",
          Color:       "#f97316",
          MenuPath:    []string{"My Plugin", "Requests"},
          DefaultConfig: map[string]any{
            "url": "",
          },
        },
      },
    },
    Actions: map[string]pluginapi.ActionNode{
      "my_action": &myAction{},
    },
  }

  pluginsdk.Serve(bundle)
}
```

## Action Nodes

Action nodes implement:

```go
ValidateConfig(ctx context.Context, config json.RawMessage) error
Execute(ctx context.Context, config json.RawMessage, input map[string]any) (any, error)
```

Use action nodes for normal pipeline steps such as:

- calling a custom API
- talking to a database or internal service
- transforming data with custom Go logic
- branching on service-specific results

### Action Config Rendering

It helps to think about action config in two phases:

- `ValidateConfig` receives the raw saved JSON config.
- `Execute` receives config after Emerald renders template strings inside it.

That means:

- validate required keys, JSON shape, and obvious bad values in `ValidateConfig`
- do not expect `{{input.*}}`, `{{secret.*}}`, or `{{$('node-id').*}}` placeholders to be resolved during `ValidateConfig`
- expect plain rendered values in `Execute`

The `input` argument passed to `Execute` is the current node payload. It is the same object users can reference in templates as `input`.

### Declaring Fields

`pluginapi.FieldSpec` drives the generic config UI. Available field types today:

- `string`
- `textarea`
- `number`
- `boolean`
- `select`
- `json`

Useful field properties:

- `required`
- `description`
- `placeholder`
- `template_supported`
- `options`
- `default_string_value`
- `default_bool_value`
- `default_number_value`

If `template_supported` is enabled, users can put values like `{{input.foo}}`, `{{secret.api_token}}`, or `{{$('action-http-1').response.status_code}}` into that field.

Cross-node selectors only resolve after the referenced node has already executed in the current run.

### Declaring Custom Output Handles

Plugin action nodes can declare named outputs:

```go
Outputs: []pluginapi.OutputHandle{
  {ID: "success", Label: "Success", Color: "#22c55e"},
  {ID: "error", Label: "Error", Color: "#ef4444"},
},
```

When custom outputs are declared:

- the editor renders those handles on the node
- outgoing edges must use those handle IDs
- the action result must include a `matches` object containing every handle ID as a boolean

Example action result:

```json
{
  "status_code": 200,
  "body": {
    "ok": true
  },
  "matches": {
    "success": true,
    "error": false
  }
}
```

If an action declares custom outputs but returns no `matches` object, or omits a handle from `matches`, Emerald treats that as an error.

### Output Hints

`OutputHint` entries help the editor suggest useful template paths for downstream fields.

Example:

```go
OutputHints: []pluginapi.OutputHint{
  {Expression: "input.status_code", Label: "HTTP status code"},
  {Expression: "input.body", Label: "Response body"},
},
```

## Tool Nodes

Tool nodes implement:

```go
ValidateConfig(ctx context.Context, config json.RawMessage) error
ToolDefinition(ctx context.Context, meta pluginapi.ToolNodeMetadata, config json.RawMessage) (*pluginapi.ToolDefinition, error)
ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error)
```

Use tool nodes when the model should be able to call your custom capability from an `llm:agent`.

Key points:

- Tool definitions are built dynamically at runtime.
- `args` contains the model-supplied arguments for the current tool call.
- `ExecuteTool` receives the current pipeline payload in `input`.

### Tool Config Rendering Caveat

Tool config has an important v1 caveat:

- `ValidateConfig` receives raw saved config.
- `ToolDefinition` receives config after Emerald renders it without pipeline input.
- `ExecuteTool` receives config after Emerald renders it against the current pipeline payload.

In practice:

- keep fields needed for `ToolDefinition` static
- do not rely on `{{input.*}}` while building a tool definition
- prefer using runtime templates in fields that are only needed during `ExecuteTool`

If your tool definition depends on templated values, keep that behavior simple and test it carefully.

## Trigger Nodes

Trigger nodes implement:

```go
ValidateConfig(ctx context.Context, config json.RawMessage) error
```

Trigger plugins do not execute pipeline steps directly. Instead:

1. Emerald sends the plugin a full `TriggerSubscriptionSnapshot` for the active configured trigger set owned by that plugin.
2. The plugin watches its external source and emits `TriggerEvent` values back to Emerald.
3. Emerald starts the exact subscribed root node and wraps the emitted payload as that trigger node's output.

Each subscription includes:

- `subscription_id` - stable runtime key for this active trigger instance
- `pipeline_id` - owning pipeline
- `node_type` - resolved type such as `trigger:plugin/acme/inbox`
- `node_id` - plugin node ID such as `inbox`
- `node_instance_id` - concrete node ID inside the pipeline graph
- `config` - saved node config JSON for that node instance

At runtime, Emerald exposes the emitted event as normal trigger output. It also adds:

- `triggered_by = "plugin"`
- `subscription_id`
- `payload`

If the payload is an object, Emerald also flattens its top-level keys onto the trigger output for easier downstream templating.

## Trigger Runtime Stream

Trigger plugins expose their runtime stream through `OpenTriggerRuntime`.

Practical guidance:

- Treat each snapshot as authoritative replacement state, not an incremental patch.
- Rebuild or refresh your in-plugin watchers when the snapshot changes.
- Emit only JSON-compatible payloads.
- Include enough payload fields that downstream actions can template without extra unpacking.

## Secrets

Emerald has a global encrypted secret store. Secret values are available at runtime through the reserved `secret` object:

```text
{{secret.api_token}}
{{secret.database_password}}
```

Useful details:

- Secrets are created in the UI under `Settings`.
- Secret values are injected into runtime context but are not stored in normal execution context records.
- List and metadata APIs return secret names and timestamps, not plaintext values.
- Action node config can use secrets the same way built-in nodes do.
- Template-enabled fields can also read earlier node results with `{{$('node-id').path}}` when that node has already run in the current execution path.

Because tool definitions are built before normal runtime input is available, keep secret-dependent tool config limited to fields that matter during `ExecuteTool`, not fields required to build the tool schema.

## Sample Plugin

The repository includes a reference plugin at:

[`examples/plugins/sample-request-kit`](/H:/Projects/Go/src/github.com/FlameInTheDark/emerald/examples/plugins/sample-request-kit)

It demonstrates:

- one custom action node
- one custom tool node
- custom fields
- custom output handles
- output hints
- plugin bundle structure

The repository also includes a trigger-focused reference plugin at:

[`examples/plugins/sample-trigger-kit`](/H:/Projects/Go/src/github.com/FlameInTheDark/emerald/examples/plugins/sample-trigger-kit)

It demonstrates:

- a plugin-defined trigger node
- trigger runtime subscription snapshots
- emitted trigger events with JSON payloads
- downstream-friendly trigger output fields

Build it from the repository root:

```powershell
go build -o .\examples\plugins\sample-request-kit\bin\sample-request-kit.exe .\examples\plugins\sample-request-kit
```

Then copy or keep the bundle under your plugin root, for example:

```text
.agents/plugins/sample-request-kit/
```

After restarting Emerald, you should see the plugin nodes in the normal `Actions` and `Tools` palettes.

## Practical Authoring Tips

- Keep `NodeSpec.ID` stable once users may already have pipelines that reference it.
- Use `MenuPath` to group related plugin nodes under a service or feature area instead of dropping everything into one long category list.
- Return plain JSON-compatible objects from actions and tools.
- Validate required config fields early and return friendly errors.
- Prefer explicit output field names over deeply nested ad hoc payloads.
- Use any Lucide icon name for `NodeSpec.Icon`. Kebab-case names such as `globe`, `database`, `server`, `cloud`, or `message-square` are the clearest choice.
- Build one action node first, get it working in the editor, then add tool nodes and custom outputs.
- For trigger plugins, start with one subscription type and make snapshot replacement logic explicit before you add more external sources.

## Troubleshooting

### Plugin Does Not Show Up

Check:

- the bundle is under the resolved plugin root
- `plugin.json` exists
- the `executable` path is correct
- the binary is built for the current host OS and architecture
- you restarted Emerald after adding the manifest or rebuilding the binary

### Plugin Shows As Unavailable

Check:

- plugin startup logs or reported error text
- API version mismatch with `pluginapi.APIVersion`
- duplicate plugin ID
- duplicate node IDs or duplicate resolved node types
- manifest `id` and runtime `PluginInfo.ID` mismatch

### Custom Outputs Do Not Route

Check:

- the node declares `Outputs`
- each edge uses one of those handle IDs
- the action result includes `matches`
- every declared handle is present in `matches` with a boolean value

### Tool Node Fails Before Execution

Check:

- whether the tool config contains templates needed during `ToolDefinition`
- whether a field used to build the tool schema depends on `{{input.*}}`
- whether a secret placeholder was placed in a field that Emerald tries to render before `ExecuteTool`

When in doubt, keep tool-definition inputs static and move dynamic values into execution-time fields.
