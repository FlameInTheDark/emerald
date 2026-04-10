# Node Reference

Emerald pipelines are built from typed nodes. Every executable node receives a runtime payload, can read values from that payload in templates or expressions, and may produce a new payload for downstream nodes.

## How Nodes Behave

- Trigger nodes start a pipeline.
- Action nodes run on the normal execution path.
- Logic nodes branch, merge, aggregate, or return data.
- LLM nodes either send a single prompt or run an agent with connected tools.
- Tool nodes are only meant to be connected to `llm:agent` nodes.
- Visual nodes affect the canvas only and never execute.

## Runtime Payload Basics

- In templates, `input` means the whole current payload for the node.
- Top-level payload keys are also exposed directly, so `{{status_code}}` and `{{input.status_code}}` can both work when that key exists.
- Logic nodes often wrap upstream data instead of passing it through unchanged.

Example:

- If an HTTP node outputs `response`, then a directly connected prompt node can read `{{input.response}}`.
- If you insert `logic:switch` between them, the switch stores the upstream payload under its own `input` field, so the downstream template path becomes `{{input.input.response}}`.

## Node Families

### Triggers

| Type | Label | Purpose | Notes |
| --- | --- | --- | --- |
| `trigger:manual` | Manual Trigger | Start the pipeline from the editor or API manually. | Useful for testing and ad hoc runs. |
| `trigger:cron` | Cron Trigger | Start the pipeline on a schedule. | Uses cron syntax plus a configured timezone. |
| `trigger:webhook` | Webhook Trigger | Represent an HTTP-triggered pipeline start. | A node type exists in the catalog, but the current server build does not yet expose a public webhook route. |
| `trigger:channel_message` | Channel Message | Start the pipeline when a Telegram or Discord user sends a message. | Requires a configured channel integration. |

### Actions

### Local and Workflow Actions

| Type | Label | Purpose |
| --- | --- | --- |
| `action:http` | HTTP Request | Make an HTTP request from the main pipeline flow. |
| `action:shell_command` | Shell Command | Run a local command in the current workspace or a configured directory. |
| `action:lua` | Lua Script | Run in-process Lua for compact transformations or glue logic. |
| `action:pipeline_get` | Get Pipeline | Load pipeline metadata and optionally its definition. |
| `action:pipeline_run` | Run Pipeline | Execute another pipeline and pass JSON parameters. |

### Messaging Actions

| Type | Label | Purpose |
| --- | --- | --- |
| `action:channel_send_message` | Send Channel Message | Send a message through a configured channel. |
| `action:channel_reply_message` | Reply To Message | Reply to an existing message. |
| `action:channel_edit_message` | Edit Channel Message | Edit a previously sent message by ID. |
| `action:channel_send_and_wait` | Send And Wait | Send a message and wait for the user's reply. |

### Proxmox Actions

| Type | Label | Purpose |
| --- | --- | --- |
| `action:proxmox_list_nodes` | List Nodes | List cluster nodes. |
| `action:proxmox_list_workloads` | List VMs/CTs | List virtual machines and containers. |
| `action:vm_start` | Start VM | Start a Proxmox VM. |
| `action:vm_stop` | Stop VM | Stop a Proxmox VM immediately. |
| `action:vm_clone` | Clone VM | Clone a Proxmox VM with a new ID and name. |

### Kubernetes Actions

| Type | Label | Purpose |
| --- | --- | --- |
| `action:kubernetes_api_resources` | K8s API Resources | List API resources available on the cluster. |
| `action:kubernetes_list_resources` | K8s List Resources | List resources by apiVersion and kind or by resource name. |
| `action:kubernetes_get_resource` | K8s Get Resource | Fetch one resource. |
| `action:kubernetes_apply_manifest` | K8s Apply Manifest | Apply a manifest with server-side apply. |
| `action:kubernetes_patch_resource` | K8s Patch Resource | Patch a resource. |
| `action:kubernetes_delete_resource` | K8s Delete Resource | Delete a named resource or a matching collection. |
| `action:kubernetes_scale_resource` | K8s Scale Resource | Update workload replica count. |
| `action:kubernetes_rollout_restart` | K8s Rollout Restart | Restart a rollout-capable workload. |
| `action:kubernetes_rollout_status` | K8s Rollout Status | Wait for rollout readiness. |
| `action:kubernetes_pod_logs` | K8s Pod Logs | Read pod logs. |
| `action:kubernetes_pod_exec` | K8s Pod Exec | Run a command in a pod. |
| `action:kubernetes_events` | K8s Events | Read recent Kubernetes events. |

### Tools

Tool nodes are not part of the normal left-to-right execution path. They are connected to the bottom `tool` output of an `llm:agent` node and are exposed to the model as callable tools.

Emerald includes tool variants for these families:

- Proxmox: `tool:proxmox_list_nodes`, `tool:proxmox_list_workloads`, `tool:vm_start`, `tool:vm_stop`, `tool:vm_clone`
- Local: `tool:http`, `tool:shell_command`
- Pipeline management: `tool:pipeline_list`, `tool:pipeline_get`, `tool:pipeline_create`, `tool:pipeline_update`, `tool:pipeline_delete`, `tool:pipeline_run`
- Messaging: `tool:channel_send_and_wait`
- Kubernetes: `tool:kubernetes_api_resources`, `tool:kubernetes_list_resources`, `tool:kubernetes_get_resource`, `tool:kubernetes_apply_manifest`, `tool:kubernetes_patch_resource`, `tool:kubernetes_delete_resource`, `tool:kubernetes_scale_resource`, `tool:kubernetes_rollout_restart`, `tool:kubernetes_rollout_status`, `tool:kubernetes_pod_logs`, `tool:kubernetes_pod_exec`, `tool:kubernetes_events`

### Logic

### `logic:condition`

- Evaluates one boolean expression.
- Exposes two source handles: `true` and `false`.
- Output shape:

```json
{
  "condition": "input.status == \"ready\"",
  "input": { "...": "original payload" },
  "result": true
}
```

### `logic:switch`

- Evaluates one or more named boolean expressions.
- Exposes one source handle per configured condition plus a `default` handle.
- Fans out to every matching condition, not only the first match.
- Output shape:

```json
{
  "conditions": [],
  "matches": {
    "condition-1": true,
    "default": false
  },
  "matched": [],
  "matchedCount": 1,
  "hasMatch": true,
  "defaultMatched": false,
  "input": { "...": "original payload" }
}
```

### `logic:merge`

- Reads the outputs of all incoming nodes from execution context.
- Supports `shallow` and `deep` merge modes.
- Output includes the merged object plus metadata such as `merged`, `mode`, `count`, `entries`, and optional `extras` for non-object upstream outputs.

### `logic:aggregate`

- Collects all incoming outputs without merging them.
- Output shape:

```json
{
  "count": 2,
  "items": [],
  "entries": [],
  "byNodeId": {}
}
```

- `idOverrides` can rename keys inside `byNodeId`.

### `logic:return`

- Stops the current pipeline and returns a value to the caller.
- If `value` is empty, it returns the whole current payload.
- Output shape:

```json
{
  "status": "returned",
  "value": {}
}
```

### LLM

### `llm:prompt`

- Sends one rendered prompt to the selected provider.
- Best for single-step generation or summarization.
- Output includes provider metadata, the rendered prompt, `content`, optional `toolCalls`, usage information, and `status`.

### `llm:agent`

- Runs a multi-turn tool-using agent.
- Discovers connected tool nodes dynamically at runtime.
- Can optionally expose workspace skills.
- Output includes `content`, `toolCalls`, `toolResults`, `tools`, usage information, and `status`.

### Visual

### `visual:group`

- Canvas-only grouping container.
- Helps organize related nodes visually.
- Does not affect execution.

### Plugin Nodes

Plugin nodes appear in the existing `Actions` and `Tools` categories.

- Action plugins use node types shaped like `action:plugin/<plugin-id>/<node-id>`.
- Tool plugins use node types shaped like `tool:plugin/<plugin-id>/<node-id>`.
- Plugin action nodes can declare custom output handles.
- Plugin tool nodes behave like built-in tools and are meant to be connected to `llm:agent`.

For authoring details, see the [plugin guide](../plugins/README.md).
