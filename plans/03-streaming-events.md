# Plan 03 — Streaming & Typed Event Bus

**Goal:** turn the single-topic `RunEvent` bus into a typed multi-topic event system and expose the four streaming endpoints the CLI needs: `trace/tail`, `events/tail`, `logs/tail`, plus the existing `spans/stream`. Wire contract (JSONL envelope + heartbeat + StreamClosed) is the same across all streams.

## Read first

- Current bus: `core/bus/bus.go` — one type `RunEvent`, fan-out with drop-on-slow. Keep the drop semantics, replace the single type with a typed envelope.
- Current SSE: `server/internal/httpbridge/streams.go` — SSE with keepalive comment every 30s. Good template, but the CLI spec wants **JSONL** (one JSON object per line), not SSE `data:` frames, for `*/tail` endpoints. Keep SSE only for `spans/stream` (dashboard uses it); new streams are JSONL.

## Design

### 1. Typed event bus
`core/bus/bus.go` — replace `RunEvent` with a generic `Event`:

```go
type Event struct {
    Kind      string          // "run.started", "span.completed", "policy.updated",
                              // "budget.exceeded", "credential.rotated", "daemon.log"
    ProjectID string
    EnvID     string
    RunID     string          // optional
    AgentID   string          // optional
    SpanID    string          // optional
    At        int64            // unix millis
    Payload   json.RawMessage  // kind-specific body (already JSON-encoded)
}
```

- `Publish(Event)` unchanged (non-blocking send, drop slow subscribers).
- `Subscribe(filter Filter)` — buffered channel, apply filter at the bus so slow streams don't scan all events. Filter fields: `Kinds []string` (prefix-match, e.g. `"span."`), `ProjectID`, `EnvID`, `RunID`. Empty filter = everything.
- Keep `Subscribe() (ch, cancel)` API shape; just take a filter arg.

Migration: existing `RunEvent` publishers (trace interceptor in `server/ops/trace`, runtime server) now emit `Event{Kind:"span.completed", ...}`. Update the two call sites.

### 2. Stream envelope contract
Add `server/internal/contract/stream.go`:

```go
type StreamFrame struct {
    SchemaVersion string          `json:"schema_version"`
    Kind          string          `json:"kind"`           // "Span", "Event", "LogLine", "Heartbeat", "StreamClosed"
    Data          json.RawMessage `json:"data,omitempty"`
    At            int64           `json:"at_ms"`
    Reason        string          `json:"reason,omitempty"` // only on StreamClosed
}

func WriteFrame(w io.Writer, f StreamFrame) error  // JSON + "\n" + flush if Flusher
```

Heartbeat cadence: 15s. StreamClosed written on ctx.Done() or publisher close.

### 3. New endpoints (all JSONL, `Content-Type: application/x-ndjson`)

Put each in `server/internal/httpbridge/streams.go`:

- `GET /api/v1/trace/tail?project_id=&env_id=&run_id=` — filter kinds `run.*` + `span.*`; payload is the span/run record (use `MapSpanRowToAPI` / `MapRunToAPI`).
- `GET /api/v1/events/tail?project_id=&env_id=&kind=policy.*` — pass-through of `Event.Payload` under `StreamFrame.Data`.
- `GET /api/v1/logs/tail?level=info` — subscribes to `kind=daemon.log`. Level filter compares against `payload.level`.
- `GET /api/v1/spans/stream` — **unchanged SSE** (dashboard consumer). Just migrate its internals to the new bus (filter `kind=span.completed`).

Shared helper `streamLoop(ctx, w, ch, renderFn)` covers ctx cancel, heartbeat timer, flush, StreamClosed on exit. Don't duplicate the select loop per endpoint.

### 4. Publisher call sites
- `server/ops/trace/trace.go` — after writing a span, publish `Event{Kind:"span.completed", Payload: json(span)}`.
- `app/runtime/runtime.go` — on run create/update/cancel, publish `run.started|completed|failed|canceled`.
- New `server/internal/logsink` — small zerolog/log writer that publishes `daemon.log` events. Hook into `main.go` as a secondary `log.SetOutput` target (tee to stderr + bus).

### 5. Policy/budget/credential/agent events (stubs)
- Add `Publish` calls in `app/control/control.go` for: `PutPolicy`→`policy.updated`, `DeletePolicy`→`policy.deleted`, `CreateBudget`/`DeleteBudget`→`budget.updated`/`deleted`, credential create/delete/rotate→`credential.*`, agent create/update/delete/restore→`agent.*`.
- Payload: the updated record (MapXToAPI output). Keep it boring and uniform.

## Files

Create:
- `core/bus/bus.go` (rewrite)
- `server/internal/contract/stream.go`
- `server/internal/logsink/logsink.go`

Modify:
- `server/internal/httpbridge/streams.go` — add 3 new handlers + shared loop.
- `server/ops/trace/trace.go` — new publish signature.
- `app/runtime/runtime.go`, `app/control/control.go` — new publish calls.
- `server/main.go` — install logsink, remove unimplemented-warning lines now covered.

## Acceptance

- `curl -N localhost:PORT/api/v1/events/tail` prints JSONL frames as agents/policies are mutated in another terminal.
- `curl -N localhost:PORT/api/v1/trace/tail?run_id=<id>` prints span frames for that run only.
- `curl -N localhost:PORT/api/v1/logs/tail` prints `daemon.log` frames, including lines emitted via `log.Printf`.
- Each stream emits a `Heartbeat` frame every 15s when idle.
- Closing the client disconnects cleanly (no goroutine leak — check with `-race` test that subs map empties).
- `bus_test.go` covers: filter matches, drop-on-slow, unsubscribe, prefix kind match.
- `go build ./... && go test ./...` clean.

## Out of scope

- Historical replay (tails are live-only — backfill comes from List endpoints).
- Persisting the event stream to disk.
- OTLP export — Plan 08.
- Auth on streams — Plan 06.

## Size estimate
~500 LOC. One haiku session: bus rewrite → contract frame → migrate existing SSE → add three new tails → wire publishers → tests.
