# Slash commands

Slash commands are the TUI's verb surface. Parser lives in `slash.go`; the
canonical set of verbs and their effects in `registry.go`. Handlers in
`handlers.go` mutate the parent Model via small functions that return a
`tea.Cmd`.

## Grammar

```
SLASH    = "/" VERB (WS ARG)*
VERB     = case-insensitive [a-z]+
ARG      = unquoted | "..." | '...'  (shlex-style)
```

* Leading whitespace is ignored.
* Empty `/` is a no-op.
* `//` at the start of input is treated as message text (escape).
* Verbs are case-insensitive (`/Model` == `/model`).

## v1 verbs

| Verb         | Args                | Effect                                                  |
|--------------|---------------------|---------------------------------------------------------|
| `/help`      | —                   | Show keybinding overlay (also `?`).                     |
| `/new`       | —                   | End current chat, open a fresh one.                     |
| `/clear`     | —                   | Alias for `/new` (Claude-Code-familiar name).           |
| `/quit`      | —                   | Quit cleanly, save config.                              |
| `/exit`      | —                   | Alias for `/quit`.                                      |
| `/model`     | `[id]`              | Switch model; no arg opens picker.                      |
| `/agent`     | `[name-or-id]`      | Switch agent; no arg opens picker.                      |
| `/project`   | `[slug-or-id]`      | Switch project; no arg opens picker.                    |
| `/file`      | `<path>`            | Attach file to next message.                            |
| `/files`     | —                   | List attached files.                                    |
| `/unfile`    | `<path>`            | Remove an attached file.                                |
| `/regen`     | —                   | Regenerate last assistant response.                     |
| `/edit`      | —                   | Open last user message in composer for editing.         |
| `/copy`      | —                   | Copy last assistant message via OSC52.                  |
| `/theme`     | `[auto\|light\|dark]` | Cycle or set the color theme; persists to cliconfig.  |
| `/menu`      | —                     | Open the command palette modal (clickable + arrow-nav). |

Slash commands sent while streaming are blocked with an inline hint, **except
`/quit` and `/exit`** which cancel the stream first.

## Adding a new command

1. Append to `Verbs` in `registry.go`.
2. Add a case to `Dispatch` in `handlers.go`.
3. Add a row to the table above.
4. Cover with a model test in `slash_test.go`.
