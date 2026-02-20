# Repository Guidelines

## Project Structure & Module Organization
The entry point lives in `cmd/gelato`, which wires Cobra to the TUI runtime. Core logic sits under `internal`: `engine` owns state and snapshot logic, `server` hosts listeners and connections, `store` contains ring buffers, `ui` renders the terminal views, and `config` defines safe defaults. Shared models and helpers are under `internal/model` and `internal/testutil`. Integration-style smoke tests live in `tests/smoke`. Keep new packages scoped inside `internal` unless they are intended for reuse outside this repository.

## Build, Test, and Development Commands
- `go run ./cmd/gelato` — launch the TUI with hot code reload via `go run`. Add flags like `--no-color` for CI.
- `go build -o bin/gelato ./cmd/gelato` — produce a release-quality binary; embed version info via `-ldflags "-X main.version=..."`.
- `go test ./...` — run all unit and smoke tests, including the TCP ingestion smoke test.
- `go test ./tests/smoke -run Smoke -count=1` — quick health check for listener/engine wiring when touching networking code.

## Coding Style & Naming Conventions
Format Go code with `gofmt` (tabs for indentation) and keep imports ordered using `goimports`. Package names stay short and lowercase (`engine`, `ui`, `store`). Exported structs and methods use UpperCamelCase, while private helpers stay lowerCamelCase. Favor context-aware goroutines, explicit channel directions, and bounded buffers as reflected in `internal/engine`. Logging lines should be concise and prefixed when synthetic (e.g., `[gelato] dropping logs`). Keep files focused: one package per directory, and split large features into subfiles inside the same package.

## Testing Guidelines
Prefer table-driven tests for deterministic logic; integration behavior belongs in `tests/smoke`. When adding ingestion paths, accompany them with tests under `internal/<pkg>` plus a smoke assertion proving end-to-end flow. Aim to keep `go test ./...` under one minute by isolating heavy cases behind build tags if needed. Test names follow `TestComponentBehavior` so `go test -run Behavior` remains intuitive. Use `testutil.NewTCPSender` for socket fixtures instead of reimplementing net helpers.

## Commit & Pull Request Guidelines
Recent history uses short, imperative summaries (`ui`, `engine core`). Continue with concise commands that describe the change (`listener: enforce drop counters`). Every pull request should include: purpose statement, linked issue (if any), manual/test evidence (`go test ./...` output or screenshots of the TUI), and notes about performance implications or deployment steps. Call out backpressure or goroutine changes explicitly so reviewers can reason about scalability effects.
