# Sample Request Kit

Reference Emerald plugin bundle implemented with the Go SDK.

It includes:

- `action:plugin/sample-request-kit/branch_request`
- `tool:plugin/sample-request-kit/request_tool`

The action node demonstrates:

- custom config fields
- custom output handles (`success` and `error`)
- runtime-friendly output hints
- secret-oriented defaults such as `{{secret.api_token}}`

The tool node demonstrates:

- dynamic tool registration through the plugin runtime
- config fields that can consume secrets

## Build

From the repository root:

```powershell
go build -o .\examples\plugins\sample-request-kit\bin\sample-request-kit.exe .\examples\plugins\sample-request-kit
```

If you are building on a non-Windows host, update `plugin.json` to point at the binary name you produce.

## Try It

Copy the whole `sample-request-kit` directory under your configured plugin root, for example:

```text
.agents/plugins/sample-request-kit/
```

Then restart Emerald or refresh plugin discovery.
