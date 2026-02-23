# Gelato
<img width="500" height="500" alt="image-removebg-preview" src="https://github.com/user-attachments/assets/0215485b-e497-4121-9f0d-c6aba247f4d1" />

Gelato is a local real-time log aggregation and monitoring tool for development and internal environments.

It lets multiple services send logs to one place over TCP, then shows those logs in a live web dashboard.
Instead of tailing many terminals, you can watch all incoming logs together, filter by severity/text/source, and inspect listener/source health (connections, drops, and errors) as traffic changes.

The system is in-memory and optimized for fast ingestion and live viewing, with a Go backend and a React frontend.

## What it does

- Ingests newline-delimited logs from many TCP clients.
- Tracks listeners and source-level activity (connections, drops, errors).
- Classifies log levels (`debug`, `info`, `warn`, `error`) from message text.
- Streams snapshot and delta updates to the browser over WebSocket.
- Supports filtering by severity, search text, and source.

## When to use it

- You run several local services and want one live log view.
- You need quick visibility into listener and source behavior under load.
- You want lightweight observability without setting up a full logging stack.

## Architecture

- `cmd/gelato`: CLI entrypoint and runtime wiring.
- `internal/server`: high-throughput TCP ingest.
- `internal/engine`: central state, filtering, snapshots, and counters.
- `internal/web`: HTTP API + WebSocket + static frontend serving.
- `web/`: React frontend for dashboard and listener management.

## Requirements

- Go 1.21+
- Node.js 18+ (only needed when developing/building the web frontend yourself)

## Quick start

1. Start gelato:

```bash
go run ./cmd/gelato
```

2. Open the UI:

```text
http://127.0.0.1:8080
```

You should now see logs appear on the dashboard in real time.

## CLI flags

Common runtime options:

- `--web-host` (default `127.0.0.1`)
- `--web-port` (default `8080`)
- `--assets-dir` (path to built frontend assets)
- `--ws-flush-ms` (default `100`)
- `--ws-max-log-batch` (default `2000`)
- `--ws-full-snapshot-sec` (default `5`)

Example:

```bash
go run ./cmd/gelato --web-host 0.0.0.0 --web-port 8080
```

## Run options

### Local

Use this when developing or running directly from source.

1. Start gelato:

```bash
go run ./cmd/gelato
```

2. Open:

```text
http://127.0.0.1:8080
```

3. Add a listener in the Connections page and send logs to it.

### Docker

Use this when you want a containerized run with bundled frontend assets.

```bash
docker build -t gelato .
docker run --rm -p 8080:8080 gelato
```

Open `http://127.0.0.1:8080`.
