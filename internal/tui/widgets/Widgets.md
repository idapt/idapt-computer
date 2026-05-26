# widgets

Reusable Bubble Tea components used by the TUI Model.

| Widget       | Wraps              | Responsibility                                           |
|--------------|--------------------|----------------------------------------------------------|
| `Composer`   | `bubbles/textarea` | Multi-line input, attached-file chips, paste handling.   |
| `Transcript` | `bubbles/viewport` | Scrolling message list, optimistic update, finalize.     |
| `Header`     | (custom)           | Single-line context strip: workspace · model · agent.      |
| `Status`     | (custom)           | Single-line bottom strip: tokens, cost, state, hint.     |
| `Picker`     | `bubbles/list`     | Modal filterable list overlay for model/agent/workspace.   |

Each widget exposes:

* `Init() tea.Cmd` — focus / initial fetch where applicable.
* `Update(tea.Msg) (Widget, tea.Cmd)` — handles its own messages.
* `View() string` — pure render.
* `SetSize(w, h int)` — called from the parent on `WindowSizeMsg`.

State invariants are documented at the top of each widget file.
