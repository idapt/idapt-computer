# SSE client

`internal/api/sse.go` is the CLI's SSE consumer. It is shared by the TUI, the
`chat ask --stream` / `chat send --stream` subcommands, and any future
streaming surface.

## Surface

```go
r, err := client.StreamSSE(ctx, "POST", "/api/v1/chats/{id}/messages", body,
    api.WithResume(lastEventID),    // optional
    api.WithHeartbeat(45*time.Second), // optional
)
for {
    ev, err := r.Next()
    if errors.Is(err, io.EOF) { break }
    if errors.Is(err, api.ErrHeartbeatTimeout) { /* reconnect */ }
    // …
}
_ = r.Close()
```

* `r.LastEventID()` returns the most recent `id:` seen on the wire. Pass it to
  the next `StreamSSE` call's `WithResume` to recover events the client missed
  during a disconnect. The server semantics mirror the web client's resume
  contract — see `lib/sse/sse.md` and
  `lib/sse/client/heartbeat-watchdog.ts`.

* `WithHeartbeat(d)` enables a per-stream watchdog: if `Next()` goes `d`
  without observing an event, it returns `ErrHeartbeatTimeout`. SSE keepalive
  comments (`: …`) count as transport activity, so chatty servers won't false-
  positive. The TUI uses 45 s to mirror the web client.

* `Close()` is idempotent and safe to call from another goroutine — it unblocks
  an in-flight `Next()` by closing the underlying body.

## Sentinels

| Sentinel              | Meaning                                                              |
|-----------------------|----------------------------------------------------------------------|
| `io.EOF`              | Server ended the stream cleanly.                                     |
| `ErrHeartbeatTimeout` | No event observed in the configured heartbeat window.                |
| `ErrStreamClosed`     | `Close()` was called and a later `Next()` was issued.                |
| `*APIError`           | The connection itself failed pre-stream; see `errors.go`.            |

## Reconnect pattern (TUI)

```go
for {
    r, err := client.StreamSSE(ctx, method, path, body,
        api.WithResume(lastEventID), api.WithHeartbeat(45*time.Second))
    if err != nil { /* backoff or surface */ break }
    for {
        ev, err := r.Next()
        if ev != nil { handle(ev) }
        if errors.Is(err, io.EOF) { return }
        if err != nil {
            lastEventID = r.LastEventID()
            _ = r.Close()
            break // outer loop reconnects with lastEventID
        }
    }
}
```

The reconnect loop lives in `internal/tui/stream/reconnect.go`; this package
only exposes the primitives. Other callers (e.g. `chat send --stream`) can
adopt the same loop or accept a one-shot stream that fails fast.
