# stream

SSE → `tea.Msg` bridge for the TUI.

The bridge spawns one goroutine per stream that drains `api.SSEReader.Next()`
and pushes typed messages into a buffered channel. The Model reads the
channel via a recursive `ReadNext` `tea.Cmd` — Bubble Tea processes one msg
per tick, so a buffered channel + recursive read lets the SSE goroutine keep
making progress while the UI catches up.

## Messages

| Type             | Purpose                                            |
|------------------|----------------------------------------------------|
| `ChunkMsg`       | One chunk of streamed text.                        |
| `DoneMsg`        | Stream completed cleanly with final cost / tokens. |
| `ErrMsg`         | Stream failed (auth, network, server error).       |
| `ReconnectingMsg`| Surfaced while the reconnect loop retries.         |

## Reconnect

`reconnect.go` owns the loop: on disconnect we sleep with exponential
backoff (500ms → 1s → 2s → 5s, capped at 5 retries) and call StreamSSE
again with `WithResume(lastEventID)`. Mid-stream chunks arriving with a
stale `messageID` (because the client cancelled then reconnected) are
dropped at the Model layer.

## API contract

`Start(ctx, Params) tea.Cmd` is the only exported entry point. It returns
a Cmd that emits `ReadyMsg` (carrying the channel) immediately followed
by `ChunkMsg`s, terminated by `DoneMsg` or `ErrMsg`.

Hard rule: no caller of this package imports `net/http` — all networking
goes through `internal/api/`.
