<div align="center">

<img src="assets/node-editor-gem-network-transparent.svg" alt="Emerald visual automation overview" width="100%" />

<br/>

[![Docs](https://img.shields.io/badge/Docs-Project%20Guide-0f172a?style=for-the-badge)](docs/README.md)
[![Nodes](https://img.shields.io/badge/Docs-Node%20Reference-059669?style=for-the-badge)](docs/nodes/README.md)
[![Templates](https://img.shields.io/badge/Docs-Templates-f59e0b?style=for-the-badge)](docs/templates/README.md)
[![Plugins](https://img.shields.io/badge/Docs-Plugins-818cf8?style=for-the-badge)](docs/plugins/README.md)
[![Changelog](https://img.shields.io/badge/Project-Changelog-475569?style=for-the-badge)](CHANGELOG.md)

<br/>

**Single-binary visual automation platform written in Go.**

Node-based pipelines | Infrastructure automation | Channel workflows | Local tooling | LLM agents

<br/>

[Documentation](docs/README.md) | [Changelog](CHANGELOG.md) | [Docker](#docker) | [Development](#development)

</div>

---

## ![](docs/assets/icons/layout-dashboard.svg) Architecture

Emerald packages the backend API, embedded React UI, SQLite persistence, websocket updates, scheduler, local skill loading, plugin runtime, and node execution engine into one deployable binary. You build pipelines visually, run them manually or from triggers, watch execution progress live, and iterate without splitting the stack across multiple services.

| Component | Path | Responsibility |
| :-- | :-- | :-- |
| Server entrypoint | `cmd/server` | Starts the web server, CLI, embedded UI, and shared runtime bootstrap |
| API and embedded frontend | `internal/api` | Fiber API, auth, settings, embedded web assets, websocket endpoints |
| Pipeline runtime | `internal/pipeline` | Graph parsing, validation, execution, progress tracking, sub-pipeline runs |
| Node system | `internal/node` | Built-in triggers, actions, tools, logic, Lua, and LLM nodes |
| Persistent stores | `internal/db` | SQLite models, queries, encryption-backed config and secret storage |
| Integrations | `internal/channels`, `internal/llm` | Telegram, Discord, Proxmox, Kubernetes, and LLM provider support |
| Extensibility | `internal/plugins`, `internal/skills` | Local plugin loading and workspace skill discovery |

---

## ![](docs/assets/icons/zap.svg) Features

- Visual pipeline editor with triggers, actions, tools, logic, Lua, and LLM nodes
- Manual, cron, and channel-triggered execution paths
- Built-in Proxmox and Kubernetes automation nodes and agent tools
- HTTP, shell command, Lua, messaging, and sub-pipeline execution nodes
- Public webhook trigger endpoints at `/webhook` and `/webhook/<path>`
- Data transformation nodes for merge, aggregate, sort, limit, deduplicate, summarize, and return
- LLM prompt and agent nodes with connected tool-node support
- Built-in chat workspace for working directly with configured LLM providers
- Node editor assistant with `Ask` and `Edit` modes for live graph inspection and in-memory edits
- Local workspace skills loaded from the nearest `.agents/skills` directory
- Plugin support for custom action, tool, and trigger nodes
- Live execution tracking over websocket plus CLI pipeline progress
- Built-in authentication, users, encrypted secrets, and configuration management

---

## ![](docs/assets/icons/layers.svg) Stack

| Area | Technology |
| :-- | :-- |
| Language | Go `1.26.1` |
| Backend | Fiber v2 |
| Frontend | React + Vite (embedded into the server binary) |
| Storage | SQLite |
| Pipeline runtime | Custom node graph engine with cron scheduling |
| Infrastructure integrations | Proxmox VE, Kubernetes |
| Messaging integrations | Telegram, Discord |
| AI integrations | OpenAI, OpenRouter, Ollama, OpenAI-compatible endpoints |
| Extensibility | HashiCorp go-plugin, local skill bundles |
| CLI UX | `urfave/cli/v3`, `go-pretty` |

---

## ![](docs/assets/icons/rocket.svg) Getting Started

### Prerequisites

- Go `1.26.1` or newer
- Node.js `20+`
- `npm`
- `make`
- A CGO-capable C toolchain for `github.com/mattn/go-sqlite3`

### Quick start

```bash
make build
make run
```

Open [http://localhost:8080](http://localhost:8080).

Default login credentials for a fresh database:

- Username: `admin`
- Password: `admin`

Change the default password after first login in `Settings -> Security -> Users`.

---

## ![](docs/assets/icons/terminal.svg) Running And CLI

Run the app directly:

```bash
make run
```

Or start it with Go:

```bash
go run ./cmd/server
```

Useful CLI commands:

| Command | What it does |
| :-- | :-- |
| `emerald` | Start the web server with the default config |
| `emerald server --port 9000` | Start the server on a custom port |
| `emerald server --host 127.0.0.1` | Start the server with a custom host value |
| `emerald pipeline list` | List pipelines |
| `emerald pipeline get --id <id>` | Show a pipeline by ID |
| `emerald pipeline executions --id <pipeline-id>` | Show recent executions for a pipeline |
| `emerald pipeline execution --id <execution-id>` | Show execution details and node logs |
| `emerald pipeline run --id <id> --input '{"foo":"bar"}'` | Manually run a pipeline with JSON input |
| `emerald config list` | List stored clusters, channels, and LLM providers |
| `emerald config get --resource llm_provider --name "Primary Provider"` | Show one config item |
| `emerald config update --resource proxmox_cluster --id <id> --patch '{"host":"pve.internal"}'` | Update one config item |
| `emerald debug sql` | Open the interactive SQL shell |
| `emerald db migrate` | Apply database migrations |
| `emerald db version` | Show the current database schema version |

The `config get` and `config update` commands redact sensitive values by default. Use `--show-secrets` only when you explicitly need decrypted values in CLI output.

### Interactive SQL shell

```text
emerald-sql> .help
Available commands:
  .tables
  .schema <table>
  .help
  .exit, .quit
```

---

## ![](docs/assets/icons/server.svg) Docker

Start Emerald with Docker Compose:

```bash
docker compose up --build -d
```

The Compose stack stores SQLite data in the `emerald-data` named volume and persists the container-local `.agents` directory in the `emerald-agents` named volume so bundled and custom skills survive restarts.

Published images are expected at:

```text
ghcr.io/flameinthedark/emerald:<tag>
ghcr.io/flameinthedark/emerald:latest
```

---

## ![](docs/assets/icons/layers.svg) Configuration

Emerald reads configuration from environment variables.

| Variable | Default | Description |
| :-- | :-- | :-- |
| `EMERALD_PORT` | `8080` | HTTP server port |
| `EMERALD_HOST` | `0.0.0.0` | Host value loaded into config |
| `EMERALD_DB_PATH` | `./emerald.db` | SQLite database path |
| `EMERALD_AUTH_USERNAME` | `admin` | Username ensured at startup when it does not already exist |
| `EMERALD_AUTH_PASSWORD` | `admin` | Password used when creating the bootstrap user |
| `EMERALD_AUTH_SESSION_TTL_HOURS` | `24` | Session lifetime in hours |
| `EMERALD_AUTH_COOKIE_NAME` | `emerald_session` | Authentication cookie name |
| `EMERALD_ENCRYPTION_KEY` | empty | Optional 32-character seed used to encrypt stored secrets |
| `EMERALD_SKILLS_DIR` | empty | Optional override for the local skills directory |
| `EMERALD_PLUGINS_DIR` | empty | Optional override for the local plugins directory |

Notes:

- If `EMERALD_ENCRYPTION_KEY` is not provided, Emerald generates one on first boot and stores it in the database.
- Stored secrets include cluster credentials, channel tokens, and LLM provider keys.
- If `EMERALD_SKILLS_DIR` is not set, Emerald searches upward from the current working directory and executable location for the nearest `.agents/skills`, then falls back to `./.agents/skills`.
- If `EMERALD_PLUGINS_DIR` is not set, Emerald uses the local `.agents/plugins` path relative to the workspace or executable.

---

## ![](docs/assets/icons/activity.svg) AI Experiences

### Chat workspace

- Persistent browser conversations with configured LLM providers
- Interruptible responses so users can stop generation in progress
- Local skills summarized into chat context and loadable through built-in tooling
- Shared assistant base instructions configurable in `Settings -> AI -> Assistants`

### Node editor assistant

- `Ask` mode for read-only questions about the current unsaved pipeline snapshot
- `Edit` mode for validated in-memory graph edits before saving
- Canvas locking during live edit application to keep browser state consistent
- Shared assistant base instructions configurable in `Settings -> AI -> Assistants`

---

## ![](docs/assets/icons/terminal.svg) Development

Build the embedded frontend and server:

```bash
make build
```

Work on the frontend separately with Vite:

```bash
cd web
npm ci
npm run dev
```

Useful Make targets:

| Target | What it does |
| :-- | :-- |
| `make build-web` | Build the React app into `internal/api/web/dist` |
| `make build` | Build the frontend and Go server binary |
| `make run` | Run the server with the embedded frontend |
| `make test` | Run `go test -race -cover ./...` |
| `make lint` | Run `golangci-lint run` |
| `make clean` | Remove the built binary and embedded frontend dist |
| `make docker` | Build the Docker image |
| `make docker-run` | Start the Docker Compose stack |

---

## ![](docs/assets/icons/book-open.svg) Documentation

| | |
| :-- | :-- |
| [Documentation index](docs/README.md) | Entry point for the full project docs |
| [Node reference](docs/nodes/README.md) | Built-in node families, behavior, and payload notes |
| [Template guide](docs/templates/README.md) | `{{...}}` interpolation and runtime value lookup |
| [Expression guide](docs/expressions/README.md) | Expr-based logic and branching rules |
| [Settings guide](docs/settings/README.md) | Settings areas, navigation, and deep-link behavior |
| [Plugin reference](docs/plugins/README.md) | Plugin manifests, SDK types, runtime behavior, troubleshooting |
| [Plugin tutorial](docs/plugins/tutorial.md) | Step-by-step walkthrough for building a custom plugin |
| [Changelog](CHANGELOG.md) | Release history and notable changes |

---

## ![](docs/assets/icons/layout-dashboard.svg) Repository Layout

```text
cmd/server         server entrypoint and CLI commands
internal/api       Fiber API, handlers, auth, embedded frontend assets
internal/channels  Telegram and Discord integrations
internal/db        SQLite models, queries, migrations, secret storage
internal/llm       provider clients, chat helpers, tool integration
internal/node      built-in node registry and implementations
internal/pipeline  execution engine, validation, runtime, progress tracking
internal/plugins   plugin loading and runtime adapters
internal/skills    local skill discovery and refresh logic
docs/              project documentation
examples/plugins/  sample plugin projects
assets/            README and UI illustration assets
web/               React + Vite frontend
```

---

## ![](docs/assets/icons/activity.svg) Current Notes

- Authentication sessions are stored in memory, so restarting the server signs users out.
- Only active pipelines participate in cron scheduling and channel-triggered execution.
- Manual runs work even when a pipeline is inactive.
- Tool nodes are only meaningful when connected to an `llm:agent` node.
- Local skills are resolved from the nearest workspace `.agents/skills` by default and refreshed automatically.
- Webhook trigger routes are available publicly at `/webhook` and `/webhook/<path>`, with optional tokens accepted from `X-Emerald-Webhook-Token`, `Authorization: Bearer ...`, or `?token=...`.

---

<div align="center">

MIT License

</div>
