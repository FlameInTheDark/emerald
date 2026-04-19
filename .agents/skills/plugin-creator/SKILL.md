---
name: plugin-creator
description: Create or update Emerald custom node plugins. Use this skill when building Go-based plugin bundles that add action or tool nodes, custom config fields, output handles, menu placement, icons, and secret-friendly templated config, especially in remote environments where the Emerald repo is not checked out locally.
---

Use this skill whenever the task is to create, extend, fix, or explain an Emerald plugin bundle.

Emerald plugins are standalone Go executables plus a `plugin.json` manifest. They are not Go native `plugin` binaries and they are not Codex plugins.

## Default Source Of Truth

Assume the Emerald repository is not available locally unless the agent explicitly cloned it.

Start from Go module metadata and GitHub-hosted references:

- `go doc github.com/FlameInTheDark/emerald/pkg/pluginapi`
- `go doc github.com/FlameInTheDark/emerald/pkg/pluginsdk`
- `go list -m -json github.com/FlameInTheDark/emerald`
- [Plugin README](https://github.com/FlameInTheDark/emerald/blob/main/docs/plugins/README.md)
- [Plugin tutorial](https://github.com/FlameInTheDark/emerald/blob/main/docs/plugins/tutorial.md)
- [Sample plugin](https://github.com/FlameInTheDark/emerald/blob/main/examples/plugins/sample-request-kit/main.go)
- [pluginapi package](https://pkg.go.dev/github.com/FlameInTheDark/emerald/pkg/pluginapi)
- [pluginsdk package](https://pkg.go.dev/github.com/FlameInTheDark/emerald/pkg/pluginsdk)

Do not assume local documentation files or a local Emerald checkout exist.

If the task depends on unreleased behavior and the agent intentionally cloned Emerald, local source inspection is optional. Treat that as an edge case, not the default workflow.

## Primary Goals

- Create working plugin bundles for the real Emerald plugin system
- Use the published SDK API from the Go module, not machine-specific paths
- Keep plugin IDs, node IDs, and menu placement stable
- Make plugin nodes feel like built-in nodes in the editor and at runtime
- Verify the plugin as far as the current environment allows

## Recommended Workflow

1. Clarify the plugin shape:
   - plugin ID
   - action nodes, tool nodes, or both
   - config fields
   - whether any action node needs branching outputs
   - desired palette path such as `Actions -> Service -> Requests`
2. Choose a bundle location that makes sense for the user’s target workspace or repository.
   - Prefer a standalone folder named after the plugin ID unless the user requests a specific location.
  - Do not assume any specific local plugin directory layout.
3. Create the bundle files:
   - `plugin.json`
   - `go.mod`
   - `main.go`
   - `bin/`
4. Initialize the module and add the Emerald SDK dependency with normal Go module commands.
5. Implement the plugin with `pluginapi.Bundle` plus `pluginsdk.Serve`.
6. Build the executable into the bundle `bin/` directory.
7. If an Emerald runtime is available, rediscover plugins there.
8. Verify:
   - the code builds
   - the manifest matches the runtime metadata
   - nodes appear in the intended palette path when Emerald is available
   - execution returns the expected payload shape

## Canonical Bundle Layout

```text
my-plugin/
  plugin.json
  go.mod
  main.go
  bin/
    my-plugin
```

On Windows, the executable is typically `my-plugin.exe`.

`plugin.json` needs a stable plugin ID and an executable path that resolves from the manifest directory.

Typical manifest:

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "version": "0.1.0",
  "description": "Custom Emerald nodes for an external service.",
  "executable": "./bin/my-plugin"
}
```

If the target runtime is Windows, use `./bin/my-plugin.exe`.

## Go Module Setup

Default to released module versions. Prefer commands like:

```bash
go mod init example.com/my-plugin
go get github.com/FlameInTheDark/emerald@latest
go mod tidy
```

Plugin code should import:

- `github.com/FlameInTheDark/emerald/pkg/pluginapi`
- `github.com/FlameInTheDark/emerald/pkg/pluginsdk`

Do not default to a local `replace` directive.

Only use a local `replace` when all of these are true:

- the user explicitly wants to build against a cloned unreleased Emerald checkout
- that checkout actually exists in the environment
- the task depends on behavior not available from the published module

## Minimal Runtime Skeleton

Use `pluginapi.Bundle` unless the task truly needs a custom implementation of `pluginapi.Plugin`.

```go
bundle := &pluginapi.Bundle{
  Info: pluginapi.PluginInfo{
    ID:         "my-plugin",
    Name:       "My Plugin",
    Version:    "0.1.0",
    APIVersion: pluginapi.APIVersion,
    Nodes: []pluginapi.NodeSpec{
      {
        ID:       "my_action",
        Kind:     pluginapi.NodeKindAction,
        Label:    "My Action",
        Icon:     "globe",
        MenuPath: []string{"My Service", "Requests"},
      },
    },
  },
  Actions: map[string]pluginapi.ActionNode{
    "my_action": &myAction{},
  },
}

pluginsdk.Serve(bundle)
```

## Hard Rules

- Do not use the Go standard library `plugin` package for Emerald plugins.
- Manifest `id` and runtime `PluginInfo.ID` must match.
- `PluginInfo.APIVersion` must match `pluginapi.APIVersion`.
- `NodeSpec.ID` values must be unique within the plugin and should stay stable once pipelines may reference them.
- Use `Kind: pluginapi.NodeKindAction` for normal flow nodes and `Kind: pluginapi.NodeKindTool` for agent tools.
- Tool nodes are for `llm:agent` tool connections, not the normal execution chain.
- Plugin-defined custom outputs are supported only for action nodes.
- v1 does not support custom input pins, trigger nodes, or logic nodes from plugins.
- Return JSON-compatible values only: maps, slices, strings, numbers, booleans, or `nil`.

## Action Node Guidance

Action nodes implement:

```go
ValidateConfig(ctx context.Context, config json.RawMessage) error
Execute(ctx context.Context, config json.RawMessage, input map[string]any) (any, error)
```

Use action nodes for:

- external API calls
- database or service connectors
- domain-specific transformations
- branching behavior based on service results

Important runtime behavior:

- `ValidateConfig` receives raw saved config JSON
- `Execute` receives config after Emerald renders template strings inside it
- `input` is the current pipeline payload

That means:

- validate required keys and obvious bad values in `ValidateConfig`
- do not assume `{{input.*}}`, `{{secret.*}}`, or `{{$('node-id').*}}` is resolved in `ValidateConfig`
- expect already rendered values in `Execute`

## Tool Node Guidance

Tool nodes implement:

```go
ValidateConfig(ctx context.Context, config json.RawMessage) error
ToolDefinition(ctx context.Context, meta pluginapi.ToolNodeMetadata, config json.RawMessage) (*pluginapi.ToolDefinition, error)
ExecuteTool(ctx context.Context, config json.RawMessage, args json.RawMessage, input map[string]any) (any, error)
```

Use tool nodes when an `llm:agent` should call the capability as a tool.

Important runtime behavior:

- `ValidateConfig` receives raw saved config
- `ToolDefinition` receives rendered config before normal pipeline input is available
- `ExecuteTool` receives rendered config plus the current pipeline payload

Practical rule: keep values needed to build the tool schema static, and put dynamic behavior in `ExecuteTool`.

## `NodeSpec` Authoring Checklist

When defining a node, pay special attention to:

- `ID`: stable machine name
- `Label`: clear user-facing name
- `Description`: short explanation of what the node does
- `Icon`: any Lucide icon name
- `Color`: readable accent color
- `MenuPath`: nested palette/context-menu placement
- `DefaultConfig`: sensible starting values
- `Fields`: config form schema
- `Outputs`: custom action output pins
- `OutputHints`: template autocomplete hints for downstream nodes

`MenuPath` examples:

- `[]string{"GitHub"}` -> `Actions -> GitHub -> Node`
- `[]string{"GitHub", "Issues"}` -> `Actions -> GitHub -> Issues -> Node`
- `[]string{}` -> keep the node at the category root
- omit `MenuPath` -> Emerald places the node under the default `General` group

`Icon` can be any Lucide icon name. Prefer clear kebab-case names like `globe`, `database`, `server`, `cloud`, or `message-square`.

## Custom Config Fields

`pluginapi.FieldSpec` drives the generic node config UI.

Available field types today:

- `string`
- `textarea`
- `number`
- `boolean`
- `select`
- `json`

Useful field flags and defaults:

- `Required`
- `Description`
- `Placeholder`
- `TemplateSupported`
- `Options`
- `DefaultStringValue`
- `DefaultBoolValue`
- `DefaultNumberValue`

If a field supports templates, users can enter values like:

- `{{input.status_code}}`
- `{{input.body}}`
- `{{secret.api_token}}`
- `{{$('action:http-1775583878229').response.status_code}}`

Cross-node selectors only resolve after the referenced node has already executed in the current run.

## Custom Output Pins

If an action node declares `Outputs`, Emerald renders those source handles on the node instead of the single default output.

Example:

```go
Outputs: []pluginapi.OutputHandle{
  {ID: "success", Label: "Success", Color: "#22c55e"},
  {ID: "error", Label: "Error", Color: "#ef4444"},
},
```

When custom outputs are declared:

- outgoing edges must use those handle IDs
- the action result must include a `matches` object
- every declared handle ID must exist in `matches`
- every `matches` value must be boolean

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

If the node does not need branching outputs, omit `Outputs` and return a normal single payload.

## Secrets

Emerald has a global encrypted secret store. Plugins do not fetch secrets directly from storage. Instead, template-enabled config fields can use:

- `{{secret.api_token}}`
- `{{secret.database_password}}`
- `{{$('node-id').field}}` for data produced by an earlier node in the same run

At execution time, Emerald resolves those templates and passes the rendered value into the plugin.

Important rules:

- validate secret-backed fields by shape or presence, not by trying to read the secret yourself
- do not assume plaintext secrets are available in metadata APIs
- do not assume cross-node selectors are available unless that node has already run earlier in the same execution path
- prefer secret placeholders in `DefaultConfig` or `Placeholder` for fields like tokens, passwords, or DSNs

## Good Defaults For New Plugins

- Start with one action node before adding more nodes
- Use the sample plugin on GitHub as the structural reference
- Keep result payloads explicit and easy to template downstream
- Use friendly validation errors
- Group nodes with `MenuPath` instead of leaving a flat palette
- Add `OutputHints` when the node returns structured data users will template often

## Verification Checklist

Build the bundle:

```bash
go build -o ./bin/my-plugin .
```

On Windows:

```powershell
go build -o .\bin\my-plugin.exe .
```

Then verify as far as the current environment allows:

1. Confirm `plugin.json` matches runtime metadata.
2. Confirm the binary builds successfully.
3. If Emerald is available:
   - open `Settings`
   - use `Rediscover Plugins`
   - confirm the bundle shows up without an error
   - confirm each node appears in the intended palette path
   - confirm the node icon renders as expected
   - confirm action nodes show custom output pins when declared
   - run a small test pipeline and inspect the node output shape
4. If Emerald is not available, call out runtime discovery as a remaining verification gap instead of pretending it was checked.

## Common Mistakes

- using the wrong import path or stale pre-Emerald names
- mismatching manifest ID and runtime plugin ID
- forgetting to rebuild the executable after changing Go code
- expecting template values to be resolved during `ValidateConfig`
- building a tool definition from values that depend on `{{input.*}}`
- declaring `Outputs` but forgetting to return `matches`
- changing `NodeSpec.ID` after pipelines already reference that node
- baking in machine-specific `replace` paths or repository-relative file assumptions
- putting too many unrelated nodes at the top level instead of using `MenuPath`

## When The User Asks For A Plugin

Prefer doing the real work:

- scaffold or update the bundle
- implement the node code
- add module dependencies with normal Go commands
- build it when feasible
- rediscover plugins when feasible
- summarize what was created and any remaining verification gaps

Do not stop at generic advice unless the user clearly only wants design discussion.
