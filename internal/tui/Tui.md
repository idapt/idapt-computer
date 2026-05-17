# TUI

Bubble Tea chat interface for the idapt CLI. Boots when the user types `idapt`
in a TTY with no subcommand, or explicitly via `idapt tui`.

The TUI is a **pure presentation layer** over `internal/api/`. It never makes
HTTP calls of its own, never duplicates auth, never re-implements SSE. The
same Go client that backs `idapt chat send` backs the TUI's send loop.

## Module map

| Path | Purpose |
|---|---|
| `boot.go`              | `Run(ctx, factory)` — entry point called from `cmd/root.go` and `cmd/tui.go`. |
| `app.go`               | Root `Model`, `Init()`, helper constructors. |
| `update.go`            | `Update(msg)` — central message dispatch. |
| `view.go`              | `View()` — composes header + transcript + composer + status with lipgloss. |
| `keys.go`              | Key bindings registry (Enter, Shift+Enter, Ctrl+C, slash entry, etc.). |
| `theme.go`             | Lipgloss adaptive styles (light/dark, NO_COLOR fallback). |
| `widgets/`             | Composer, transcript, header, status, picker. See [[Widgets]]. |
| `stream/`              | SSE → tea.Msg bridge + reconnect loop. See [[Stream]]. |
| `markdown/`            | Glamour wrapper + partial-fence renderer. See [[Markdown]]. |
| `commands/`            | Slash command parser + handler registry. See [[Slash_Commands]]. |

## Boot flow

```
cmd/root.go RunE
   │ shouldBootTUI?  (TTY + no args + not -p / --json / --output / --quiet)
   ▼
tui.Run(ctx, *cmdutil.Factory)
   │
   ▼ build Model:
   │   - api client (from factory)
   │   - load config (last project/agent/model/chat)
   │   - fetch defaults if not persisted
   ▼
tea.NewProgram(model).Run()
   │
   ▼ on graceful quit:
   │   - save config (LastChatID etc.)
   │   - cancel any in-flight stream
   │   - restore terminal raw mode
   ▼
return error or nil
```

## Update dispatch (high level)

```
Update(msg) →
  WindowSizeMsg     → resize widgets
  KeyMsg            → composer / picker / global keymap
  sendMessageMsg    → API call + optimistic UI
  streamChunkMsg    → transcript.UpdateStreaming + 50ms throttled re-render
  streamDoneMsg     → finalize, status idle, persist LastChatID
  streamErrMsg      → render error, composer remains usable
  pickerSelectedMsg → swap context, refresh header
  slashCommandMsg   → dispatch to commands/handlers.go
  tea.QuitMsg       → save config + return
```

## Hard rules

* **No `net/http` imports** under `internal/tui/`. Enforced by
  `scripts/check-tui-no-http.sh`.
* **No duplicate state** — every visible value mirrors something in `Model`.
  React-flavored discipline: state down, events up.
* **Single stream at a time** in v1 — sends block while `streaming == true`.
* **Composer text is sacred** — never lose user input on disconnect or error.
* **Cancellation is best-effort** — late chunks after cancel are dropped silently.

## Out of scope (v1)

* Sidebar / chat-list panel.
* File-browser modal (use shell tab-completion on `/file <path>`).
* Image rendering protocols (Kitty graphics, sixel, iTerm2 inline).
* Multi-tab parallel model runs.
* Sub-agent fan-out visualization.
* `Ctrl+K` arbitrary command palette.
* Live-edit / CRDT markdown editing (web-only forever).
