# AGENTS.md
## Gelato
Single-command Go TUI log aggregator — scalable to many listeners and many concurrent connections.

This build guide defines Gelato’s architecture, scalability model (multiple IP:port bindings at once), concurrency patterns (goroutines + channels), and edge cases.

---

## 1) Product definition

### What Gelato is
Gelato is a single binary. The user runs only:

- `gelato`

A polished terminal UI opens immediately. From the UI, the operator can:
- Add and manage **multiple listeners** (multiple IPs and ports, concurrently)
- Observe log sources (remote senders) as colored, named items
- Tail logs live, filter/search, and view rolling graphs (volume/errors/drops)
- Rename/recolor/mute sources and clear buffers

### What Gelato is not
- Not a multi-command CLI (no `gelato listen`, no `gelato ui`).
- Not a heavy indexing system.
- Not a secure internet-facing ingestion platform by default.

---

## 2) Core scalability goals

Gelato must remain responsive under:
- Many listeners (dozens+ IP:port bindings)
- Many connections per listener (hundreds/thousands depending on system limits)
- High log throughput (hundreds of thousands of lines/minute)
- UI rendering load (filtering/searching on large buffers)

Scalability requirements:
- Ingestion must not be blocked by UI rendering.
- Memory must remain bounded by default.
- Backpressure behavior must be explicit (drop policy + counters).
- Connection and listener lifecycles must not leak goroutines.

---

## 3) Single-command + TUI-first UX

### Entry point
- `gelato` launches TUI.

### TUI sections (recommended)
- Dashboard (logs + graphs)
- Listeners (add/remove/inspect)
- Sources (rename/recolor/mute/clear)
- Filters (levels/search/focus)
- Help

### Listener management (in TUI)
From the Listeners view:
- `ctrl + a`: Add listener (IP/host + port; protocol shown but MVP is TCP only)
- `ctrl + d`: Stop/remove selected listener
- `ctrl + r`: Retry if in Error state

Non-blocking rule:
- Adding listeners and starting servers must not freeze the UI.
- Showing errors must be actionable (address in use, permission denied, invalid bind).

---

## 4) Cobra usage (bootstrap only)

Cobra is used only for:
- `gelato` root command
- `--help`, `--version`
Optional minimal flags (do not add more unless justified):
- `--no-color`
- `--state <path>` (optional persistence later)

All operational settings are inside the TUI.

---

## 5) Architecture overview (engine-centered)

### Components
- **TUI**: renders state, collects operator input
- **Engine**: single owner of shared state + coordinator
- **Servers**: multiple listener instances, each accepting many connections
- **Conn readers**: per-connection goroutine that reads lines and emits events

### High-level data flow
Servers produce `LogEvent`s → Engine ingests into stores/stats → TUI requests snapshots.

The UI never directly reads/writes engine-owned maps/slices.

---

## 6) Concurrency model (goroutines + channels)

### Design principle
Use goroutines aggressively where they help throughput and isolation, but keep correctness by enforcing:
- Clear ownership boundaries
- Bounded queues
- Explicit drop/backpressure policy
- Clean shutdown via context cancellation

### Goroutine topology (scalable)
- 1 goroutine: TUI runtime
- 1 goroutine: Engine main loop (owns registry/store/stats/listeners)
- For each listener:
  - 1 goroutine: accept loop
- For each accepted connection:
  - 1 goroutine: read loop (buffered line reads)
- Optional (recommended at high throughput):
  - N goroutines: ingest workers (event processing sharded)
  - 1 goroutine: stats aggregator (if stats updates are heavy)

MVP can start with single-engine ingestion, but must be structured so sharding is easy.

---

## 7) Engine internals (scalable ingestion)

### State ownership
Engine owns:
- listeners list and their status
- sources registry (names/colors/lastSeen/connCount)
- log storage ring buffers
- rolling stats buckets
- dropped counters + error counters

UI interacts only via command messages and snapshots.

### Channels
Required channels:
- `uiCmdCh` (UI → Engine): add listener, remove listener, rename source, set filters, etc.
- `snapReqCh` (UI → Engine): request snapshot for current view
- `snapRespCh` (Engine → UI): snapshot response
- `eventCh` or `eventCh[]` (Servers → Engine): ingested log events (bounded)

#### Recommended for scalability: sharded event channels
Instead of one global `eventCh`, use `K` shards:
- `eventCh[0..K-1]`, each bounded
- choose shard by `hash(sourceKey) % K`

Benefits:
- Reduced contention under load
- Enables parallel ingestion workers
- Keeps per-source event ordering stable within a shard

---

## 8) Ingestion pipeline (TCP newline delimited)

### Listener = independent accept loop
Each listener runs its own goroutine:
- `net.Listen("tcp", ip:port)`
- accept connections
- enforce `maxConns` (global and/or per-listener)
- on stop: close listener; active conns should be canceled

### Connection read loop
Each connection goroutine must:
- buffered read
- split lines on `\n` (support `\r\n`)
- enforce `maxLineBytes`
- handle partial lines on disconnect
- send `LogEvent` into the appropriate event channel shard

### Backpressure and drop policy (mandatory)
Queues are bounded. If a queue is full:
- drop the new event (do not block indefinitely)
- increment:
  - total dropped
  - per-source dropped
  - per-listener dropped (optional)
- optionally emit a synthetic internal warning log at a throttled rate:
  - `[gelato] dropping logs: event queue full`

This is required to avoid unbounded memory growth.

---

## 9) Parallel ingestion (recommended for “more goroutines” + scalability)

To scale event processing beyond a single engine loop:

### Option A (recommended): Sharded ingest workers + single state owner per shard
- Create `K` shards.
- Each shard has:
  - its own ingestion worker goroutine
  - its own subset of sources/stores/stats OR ownership partitioned by sourceKey

This is the highest throughput design but introduces complexity.

### Option B (practical): Sharded queues + batch processing in engine
- Keep single engine owner for all state.
- Use `K` event queues to reduce contention.
- Engine loop drains events in batches from all shards each tick:
  - read up to `B` events per shard per cycle
  - apply updates
This gives good scalability while preserving a single owner.

MVP should implement Option B, structured so Option A can be added later if needed.

### Batching requirement
Engine should batch:
- Append multiple events before updating UI snapshot readiness
- Update stats buckets per batch
This reduces overhead significantly.

---

## 10) Data model

### LogEvent
Minimal representation:
- `ID` (monotonic)
- `SourceKey` (remote IP or other strategy)
- `ListenerID`
- `ConnID` (optional)
- `RemoteAddr` (string)
- `ReceivedAt` (time)
- `Line` (string or bytes)
- `Level` (Debug/Info/Warn/Error/Unknown)
- `ParsedTS` (optional)
- `IngestLag` (optional)

### Listener
- `ListenerID`
- `BindIP` / `BindHost`
- `Port`
- `Status` (Starting/Listening/Error/Stopped)
- `ErrMsg`
- `ActiveConns`
- `Dropped` (optional)
- `StartedAt`

### Source identity strategy
MVP default: key by remote IP (stable across reconnects)
- Limitation: NAT merges multiple services into one source.
Mitigation:
- Track `ActiveConns` and show it in the UI.
- Later: allow “key by IP:port” toggle, or support client-provided source tag.

---

## 11) Storage and retention (bounded)

### Ring buffers
- Per-source ring buffer (default 5000 lines)
- Optional global ring buffer (default 20000 lines)
- Must be O(1) append, efficient viewport iteration

### Limits
- `maxSources` (default 500)
- If exceeded: route new sources into a special “Overflow” source (recommended)
- `maxLineBytes` (default 65536)
- `eventsQueueSizePerShard` (default e.g. 10000, multiplied by shard count)

---

## 12) Stats and terminal graphs

### Rolling windows
Maintain rolling buckets (owned/updated by engine):
- logs/sec (60×1s)
- errors/min (30×1m)
- dropped/min (30×1m)
Optionally per-source for selected source only (avoid per-source overhead for all at once).

### Level detection
Fast heuristic with boundary checks:
- ERROR/ERR → Error
- WARN → Warn
- INFO → Info
- DEBUG → Debug
If multiple match, pick highest severity.

### Lag metric (optional)
Parse timestamps only if cheap; never block ingestion.
If present, compute `ReceivedAt - ParsedTS`.

---

## 13) TUI performance requirements (so scaling doesn’t kill UI)

- Cap render FPS (10–30).
- Render only visible log lines (viewport).
- Engine provides snapshots that are already filtered to the current view when possible.
- Avoid scanning entire buffers every frame:
  - Keep current filter/search state in engine
  - Generate “render slice” for viewport only

---

## 14) Listener and connection edge cases (must handle)

Listeners:
- Bind error: address in use, permission denied, invalid IP/host
- Multiple listeners on same port but different IP (allowed)
- Stop listener with active connections (must cancel conns)
- Retry start (from Error state) without leaking goroutines

Connections:
- Too many connections (reject gracefully)
- Slow sender/stalled reads (optional read timeout)
- Disconnect mid-line (flush partial if safe)
- Oversized line (drop, drain until newline, count)
- Non-UTF8 bytes (sanitize for display)
- Queue full (drop + count; never deadlock)

Multi-listener specific:
- Same client connecting to different listeners
- Many listeners started/stopped rapidly
- Per-listener statuses must update independently and reliably

---

## 15) Shutdown and lifecycle

- Ctrl+C exits cleanly:
  - cancel root context
  - stop all listeners
  - close all active conns
  - stop engine and ingest workers
  - exit TUI without breaking terminal state

No goroutine leaks:
- Every loop selects on `ctx.Done()`.
- Listener stop closes the net.Listener and cancels all conn contexts.
- Engine owns channel closing strategy (avoid panics from sends on closed chans).

---

## 16) Configuration (defaults; set in code, adjustable later via UI)

Hard caps + safe defaults:
- default IP in add form: `127.0.0.1`
- default port: `9000`
- max line bytes: `65536`
- max sources: `500`
- per-source buffer lines: `5000`
- shards: `K = GOMAXPROCS()` (clamped to [2..16] recommended)
- queue per shard: `10000` (tuneable)
- max conns: global cap (e.g. 2000) + per-listener cap (e.g. 500)

Expose these in UI “Settings” later if needed, but keep MVP sane.

---

## 17) Implementation milestones (scalable-first)

### Milestone 1: TUI shell (single command)
- `gelato` opens TUI
- Listeners view + add form + validation (no server yet)

### Milestone 2: Multi-listener engine
- Add/remove listeners from UI
- Each listener runs accept loop goroutine
- Per-conn read goroutine emits events
- Sharded bounded event queues (K shards)
- Engine drains events in batches, updates sources + ring buffers

### Milestone 3: Logs view + filtering
- viewport rendering
- follow/pause
- level filter + search
- focus source vs all

### Milestone 4: Stats + graphs
- rolling buckets + sparklines
- drop/error counts visible and accurate

### Milestone 5: Polish + hardening
- rename/recolor/mute/clear
- terminal resize handling
- warning for 0.0.0.0 binding
- load testing and performance tuning

---

## 18) Definition of done (MVP)
- Running `gelato` launches a polished, responsive TUI.
- Operator can add **multiple** listeners (multiple IPs and ports) and they run concurrently.
- Many connections can stream logs without freezing UI.
- Bounded memory + bounded queues; drops are counted and surfaced.
- Filters/search/focus/pause behave correctly under load.
- Clean shutdown, no goroutine leaks.