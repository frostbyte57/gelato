# Agent Guidelines for gelato

## Purpose
- This file orients agentic coders to build, test, and style expectations.
- Prefer repository conventions over general Go defaults when they differ.
- Keep changes scoped and avoid touching unrelated files.

## Project Structure
- Entry point: `cmd/gelato` wires Cobra to the Bubble Tea TUI.
- Core logic: `internal/engine` (state, snapshots, filters, stats).
- Networking: `internal/server` (TCP listeners, ingest, drop stats).
- Storage: `internal/store` (ring buffers, rolling counters).
- UI: `internal/ui` (rendering, keyboard input, styles).
- Configuration: `internal/config` (safe defaults and limits).
- Shared models: `internal/model`.
- Test helpers: `internal/testutil`.
- Integration smoke tests: `tests/smoke`.
- Stress tests: `tests/stress` (longer, optional).

## Architecture Notes
- `engine.Engine` owns state, buffers, and filter logic; treat it as the source of truth.
- `server.Server` only ingests and emits events; it must remain fast and non-blocking.
- UI pulls snapshots from the engine and sends commands over buffered channels.
- Drop/error counters are maintained in the engine and updated via `EngineStatsUpdate`.
- Backpressure is handled by dropping when shard queues are full; preserve this behavior.
- Limits flow from `config.Limits`; avoid hard-coded magic numbers elsewhere.

## State and Snapshot Guidelines
- When adding fields to `engine.Snapshot`, also update UI rendering and tests.
- Keep snapshot building side-effect free (read-only over current state).
- Cache filtered lines using `filterCache` when possible to avoid rework.
- Maintain deterministic ordering for views (see sorting in `buildSources`).
- Update drop/error rate counters in the same tick as the stats update.

## Build, Run, and Release
- Go version: 1.21 (see `go.mod`).
- Run the TUI locally: `go run ./cmd/gelato`.
- Run with flags: `go run ./cmd/gelato --no-color --state=/path/state.json`.
- Build a binary: `go build -o bin/gelato ./cmd/gelato`.
- Embed version info: `go build -ldflags "-X main.version=... -X main.commit=... -X main.date=..." -o bin/gelato ./cmd/gelato`.

## Formatting and Linting
- Format Go code with gofmt: `gofmt -w ./cmd ./internal ./tests`.
- Organize imports with goimports (if installed): `goimports -w ./cmd ./internal ./tests`.
- Basic static checks: `go vet ./...`.
- Keep generated files out of git unless explicitly requested.

## Tests (All and Single)
- Run all tests: `go test ./...`.
- Run a single package: `go test ./internal/engine -run TestEngineDrainEventsProcessesAllShards -count=1`.
- Run a single test by name: `go test ./internal/server -run TestDetectLevel -count=1`.
- Run the smoke suite: `go test ./tests/smoke -run TestEngineServerSmoke -count=1`.
- Run the stress test (slower): `go test ./tests/stress -run TestStressHighVolumeIngestion -count=1`.
- Optional race check: `go test -race ./...`.

## Single-Test Tips
- Use `-run` with a regex: `go test ./internal/engine -run Filter -count=1`.
- Add `-v` for verbose output when debugging: `go test -v ./tests/smoke -run Smoke -count=1`.
- Use `-count=1` to avoid cached results during iterative changes.
- Prefer package-scoped runs over `./...` when investigating failures.

## Code Style and Conventions
- Formatting: gofmt is required; tabs for indentation.
- Imports: use goimports; order groups as standard library, third-party, then `gelato/internal/...`.
- Keep a blank line between import groups even if only two groups are present.
- Package names: short, lowercase, no underscores (example: `engine`, `ui`).
- Exported names: UpperCamelCase; unexported: lowerCamelCase.
- Acronyms: keep common acronyms uppercase (ID, UI, TCP).
- Types: use explicit types for counters (uint64) and masks (uint32); use int for sizes.
- Constants: use iota for enums and bitmasks, as in `model.Level`.
- Structs: keep data structs in `internal/model` and behavior in packages.
- Defaults: new limits should be surfaced via `config.DefaultLimits`.
- Keep files focused; split large features into subfiles in the same package.
- Avoid global mutable state outside `internal/config` defaults.

## Error Handling
- Return errors instead of panicking; reserve panic for impossible states.
- Error messages are lowercase and concise (example: `errors.New("listener not found")`).
- Add context with `fmt.Errorf("action: %w", err)` when bubbling errors up.
- On recoverable failures, update state counters and surface details in snapshots.
- Do not swallow errors in goroutines; propagate or record them.

## Concurrency and Performance
- Prefer context-aware goroutines; always listen for cancellation.
- Use buffered channels for UI or ingest paths to avoid blocking the server.
- Use explicit channel direction in public APIs (send-only/recv-only).
- Avoid unbounded fan-out; keep shard counts and queue sizes tied to limits.
- When updating drop/error stats, use atomic or engine-managed counters.
- Keep server hot paths allocation-light; avoid per-line heavy parsing.
- Respect MaxLineBytes and truncate before enqueueing.

## UI and Rendering
- `internal/ui` owns all terminal rendering and keyboard handling.
- The UI uses Unicode borders and glyphs; keep non-UI code ASCII-only.
- Preserve existing visual language (colors, badges, chips) when extending UI.
- Update status/help strings when adding new key bindings.
- Avoid blocking calls in the UI update loop; use commands instead.

## Testing Conventions
- Use table-driven tests for deterministic logic (see `server_test.go`).
- Name tests `TestComponentBehavior` to make `-run Behavior` intuitive.
- Use `config.DefaultLimits()` and override only needed fields.
- Use `testutil.NewTCPSender` for TCP fixtures.
- Use timeouts around goroutines to avoid hangs.
- Prefer `t.Fatalf` for fatal assertions and clear error messages.
- Keep smoke tests small and fast; keep stress tests isolated.
- When adding ingestion paths, add a smoke assertion proving end-to-end flow.

## Change Checklists
- Engine changes: update `engine.Snapshot` fields and any UI render paths.
- Server changes: keep `readLoop` non-blocking and update drop stats handling.
- UI changes: update key hints and the no-color rendering path.
- Config changes: keep defaults in `config.DefaultLimits` and adjust tests.
- Model changes: ensure all fields stay in sync with snapshots and views.
- New commands: extend `engine.CommandType` and handle them in UI dispatch.
- Storage changes: preserve ring buffer versioning semantics.
- Test changes: add focused unit tests plus smoke coverage when needed.
- Performance changes: call out queue, buffer, or goroutine impacts in PRs.

## Configuration and Limits
- Tune limits in `config.DefaultLimits` and keep values conservative.
- Use `runtime.GOMAXPROCS` only for deriving shard defaults.
- For tests, override limits locally instead of mutating globals.
- Document any new limit in code comments or UI labels.

## File and Package Placement
- New packages should live under `internal` unless meant for external reuse.
- Keep one package per directory; avoid mixing test-only helpers in prod dirs.
- Name files after their domain (e.g., `listener.go`, `stats.go`).

## Dependency and Module Hygiene
- Update dependencies with `go get` and keep `go.mod`/`go.sum` tidy.
- Run `go mod tidy` after adding or removing modules.
- Avoid adding new dependencies to `internal/ui` unless needed for rendering.
- Keep imports of third-party libs explicit and justified.

## Documentation Expectations
- Update this file when build or test commands change.
- Keep README lightweight; prefer concrete instructions here.
- Document new CLI flags in `cmd/gelato/main.go` help text.

## Commit and PR Notes
- Follow existing short, imperative summaries (examples: `ui`, `engine core`).
- PRs should note: purpose, tests run, and any perf/backpressure impacts.
- Call out goroutine or queue changes in PR descriptions.

## Editor and Agent Rules
- Cursor rules: none found in `.cursor/rules/` or `.cursorrules`.
- Copilot rules: none found in `.github/copilot-instructions.md`.
