# Automator

Automator is a general-purpose automation platform packaged as a single binary. It embeds a React UI into a Go backend, stores state in SQLite, and lets you build visual pipelines with triggers, actions, logic, LLM prompts, and LLM agents with tools.

Proxmox is one of the built-in integrations, alongside HTTP, shell commands, Lua, channels, local skills, and sub-pipelines.

## What it can do

- Build pipelines visually with manual, cron, webhook, and channel triggers
- Run LLM prompt and agent nodes with connected tool nodes
- Manage Proxmox clusters, nodes, storages, VMs, and CTs
- Run HTTP requests, shell commands, and Lua scripts inside flows
- Send and receive Telegram / Discord channel messages
- Call sub-pipelines and return structured data between pipelines
- Use local skills from `./.agents/skills`
- Stream execution progress and logs over websocket in the UI

## Build

The frontend is embedded into the Go binary. For a correct local build, use the Makefile:

```bash
make build
```

That command:

1. installs frontend dependencies if needed
2. builds the web app into `internal/api/web/dist`
3. compiles the Go server into `automator` or `automator.exe`

## Run locally

Run the app in local development mode:

```bash
make run
```

Or run the frontend separately while working on UI:

```bash
cd web
npm ci
npm run dev
```

The app is served on [http://localhost:8080](http://localhost:8080) by default.

## Docker

Build the image:

```bash
make docker
```

Run with Docker Compose:

```bash
docker-compose up -d
```

Published release images are intended to be pushed to:

```text
ghcr.io/flameinthedark/automator:<tag>
ghcr.io/flameinthedark/automator:latest
```

## Configuration

The server reads configuration from environment variables.

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTOMATOR_PORT` | `8080` | HTTP server port |
| `AUTOMATOR_HOST` | `0.0.0.0` | HTTP bind host |
| `AUTOMATOR_DB_PATH` | `./automator.db` | SQLite database path |
| `AUTOMATOR_ENCRYPTION_KEY` | empty | Optional key used to encrypt stored secrets |

## Development commands

```bash
make build      # build embedded frontend + server binary
make run        # run server with freshly built frontend assets
make test       # go test -race -cover ./...
make lint       # golangci-lint run
make clean      # remove built binary and embedded web dist
```

## Releases

This repository is set up for a two-stage GitHub Actions release flow:

1. `Semantic Release` runs on pushes to `main` or `master`
2. conventional commits are analyzed to create the next GitHub release and changelog
3. after a release is published, semantic-release dispatches `Release Build`
4. native binaries are built for Windows, Linux, and macOS
5. a container image is built and pushed to GHCR

Release assets are expected to include downloadable binaries for:

- Windows (`.zip`)
- Linux (`.tar.gz`)
- macOS Intel (`.tar.gz`)
- macOS Apple Silicon (`.tar.gz`)

## Notes

- Only active pipelines run cron and external trigger handlers.
- Manual runs work for both active and inactive pipelines.
- Tool nodes are only valid when connected to an `llm:agent` node via its tool handle.
- Local skills are loaded from `./.agents/skills` and are available to chat plus agent nodes when enabled.

## License

MIT
