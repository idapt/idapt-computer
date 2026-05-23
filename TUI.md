# idapt TUI

The idapt CLI ships a Bubble Tea-based interactive terminal UI. It boots
automatically when you type `idapt` with no subcommand in a terminal, or
explicitly via `idapt tui`. For non-interactive scripting use `idapt -p` /
`idapt chat ask`.

## Availability — feature flag `tui` (default **off**)

The entire TUI surface — `idapt` auto-boot, `idapt tui`, `idapt -p`, `idapt
chat ask`, and `idapt chat send --stream` — is gated on the server-side
feature flag **`tui`** (compiled-in default: `false`). The flag is opt-in
during v1 rollout so support can ramp users gradually; flipping the
server-side flag to `true` for a user enables the surface immediately on
their next CLI invocation. When OFF (the default):

* The bare `idapt` command falls through to printing help (preserving the
  pre-TUI behavior).
* `idapt tui` returns an actionable error pointing at support.
* `idapt -p` is rejected with the same error.
* `idapt chat ask` is hidden from `idapt chat --help` and refuses to run.
* `idapt chat send` (without `--stream`) continues to work — that path
  predates the TUI work and is never gated.

Flag values are cached at `~/.idapt/flags-cache.json` and refreshed every
~5 min from `GET /api/feature-flags/me`. The CLI consults the cache during
help rendering to decide visibility — fail-closed in line with `FlagHub`
and `FlagTriggers`, so a pristine install hides the TUI surface until the
flags load completes.

Server-side metadata for the flag lives in `lib/feature-flags/types.ts` —
match the opaque key `ff58` to the human label "Interactive CLI TUI" there
before flipping users on.

## Launch

```bash
idapt                          # auto-boots TUI if stdin+stdout are ttys
idapt tui                      # explicit; never auto-suppressed
IDAPT_NO_TUI=1 idapt           # disable auto-boot (CI / wrapper scripts)
idapt --output json            # any machine-readable flag also suppresses
```

## Inside the TUI

```
Enter            Send the composer's contents.
Shift+Enter      Newline (Ctrl+J on terminals that eat Shift+Enter).
Ctrl+C           Cancel current stream. If composer is empty and no stream
                 is in flight, quit. Twice within 200ms while streaming →
                 cancel + quit (escape hatch).
Ctrl+D           Quit when composer is empty.
Ctrl+L           Clear the visible transcript (chat persists in backend).
Ctrl+N           New chat.
Ctrl+P           Project picker.
Ctrl+M           Model picker.
Ctrl+G           Agent picker.
PgUp / PgDn      Scroll transcript.
?                Help overlay.
Esc              Dismiss modal / cancel stream.
Ctrl+Y           Copy last assistant message (OSC52).
```

## Slash commands

| Verb        | Args                | Effect                                            |
|-------------|---------------------|---------------------------------------------------|
| `/help`     |                     | Keybinding + slash-command reference.             |
| `/new`      |                     | End current chat, open a fresh one (alias `/clear`). |
| `/quit`     |                     | Quit and save state (alias: `/exit`).             |
| `/model`    | `[id]`              | Switch model; no arg opens the picker.            |
| `/agent`    | `[name|id]`         | Switch agent; no arg opens the picker.            |
| `/project`  | `[slug|id]`         | Switch project; no arg opens the picker.          |
| `/file`     | `<path>`            | Attach file to next message.                      |
| `/files`    |                     | List attached files.                              |
| `/unfile`   | `<path>`            | Remove an attached file.                          |
| `/regen`    |                     | Regenerate last assistant response.               |
| `/edit`     |                     | Load last user message into composer for editing. |
| `/copy`     |                     | Copy last assistant message via OSC52.            |
| `/theme`    | `[auto|light|dark]` | Cycle or set the color theme (persists).          |
| `/menu`     |                     | Open the command palette (clickable + arrow-nav). |

Slash commands sent while streaming are blocked with a hint, except
`/quit` and `/exit` which cancel the stream first.

### Argument pickers

Verbs that take a known small argument set show a **clickable + arrow-
navigable picker** when invoked without an arg — whether you typed the
slash command, clicked the toolbar button, or selected the verb from
`/menu`:

| Verb       | Without args                                      | With args                |
|------------|---------------------------------------------------|--------------------------|
| `/theme`   | Picker: Auto / Light / Dark                        | `/theme dark` (direct)   |
| `/unfile`  | Picker: list of currently-attached files           | `/unfile foo.ts`         |

Picker controls match the rest of the modal experience: arrow keys
navigate, type to filter, Enter selects, click selects, Esc dismisses.
The cascade case works too — selecting `/theme` from the `/menu`
palette opens the theme-arg picker immediately after.

Model / agent / project pickers (which fetch from the API) keep their
existing flow.

### `/model` picker — cached + recent-first

Opening `/model` (or the `[Model]` toolbar button) renders a cached
catalog of every model your account can use. Source of truth is
`GET /api/v1/models`; the response is cached locally so the modal opens
instantly on repeat visits.

| Concern | Behavior |
|---|---|
| Cache location | `~/.idapt/models-cache.json` |
| TTL            | 15 minutes (`models.CacheTTL`) |
| Cache key      | SHA-256 of `(APIURL, APIKey)` — different keys / environments never share entries. |
| Stale fallback | If the network fetch fails and a stale entry exists, we serve the stale list rather than render a blank modal. |
| Force refetch  | `Ctrl+R` inside the picker bypasses the cache and re-hits the endpoint. |
| Recent-first   | The five most-recently-selected models (per `cfg.RecentModelIDs`) bubble to the top with a `★ recent` prefix in their subtitle. |
| Subtitle       | `<provider> · <ctx>k ctx · $<in>/$<out> · 🛠 👁 🧠` — capability glyphs are dropped silently when the server reports an unknown flag. |
| Locked rows    | RBAC-denied entries render dim + non-selectable with the server-provided reason. |

The cache is best-effort: torn JSON, missing file, or future-version
schema all degrade silently to a blocking fetch. Concurrent saves are
atomic-rename so racing `idapt` processes can't corrupt each other's
cache.

### Autocomplete

Typing `/` opens a floating suggestion menu above the composer with the
filtered verb list (canonical commands **plus** their aliases — `/clear`
and `/exit` show up alongside `/new` and `/quit`).

| Key       | Effect                                                                                  |
|-----------|-----------------------------------------------------------------------------------------|
| ↑ / ↓     | Move selection.                                                                         |
| Tab       | Complete the slash word in place. Trailing text and the rest of the buffer are kept.    |
| Enter     | Complete the slash word in place AND submit the buffer.                                 |
| Esc       | Dismiss the menu; the buffer is unchanged.                                              |
| Click     | Same as Enter on the clicked row.                                                       |

**Completion rules** (the autocomplete is intentionally conservative):

* The menu **only triggers when the cursor is INSIDE the in-progress
  slash word**. Click somewhere else in the buffer and it hides — Tab /
  Enter won't accidentally rewrite a span you didn't mean to touch.
* When the buffer's slash word is **already an exact match** for a
  registered verb (e.g. you've typed all of `/help`), the menu hides
  silently — there's nothing to suggest, and submitting on Enter just
  sends the buffer.
* Completion **rewrites only the slash-word span**. Anything you typed
  after it (`/he` → Tab → `/help foo bar` → still has `foo bar`) is
  preserved. The cursor lands at the end of the inserted verb, or after
  a single space when the verb takes arguments.
* Literal slash messages are escaped with `//` — typing `//literal /text`
  sends the message body as-is; the menu is hidden whenever the buffer
  starts with `//`.

**Recognition badge**: when the buffer's first line starts with a
registered slash verb (regardless of where the cursor is now), the
composer's prompt symbol switches from `>` to `▸` and the border tints
to the theme's primary blue. This is the at-a-glance "your command is
real, the rest is arguments" cue. It stays lit while you fill in args
and clears the moment the verb word stops matching.

## Theming

Three modes, switched via `/theme`:

* **`auto`** (default) — `lipgloss.AdaptiveColor` picks per-cell at render
  time based on the terminal's detected background.
* **`light`** — forces the light side of every adaptive pair.
* **`dark`** — forces the dark side.

Persists to `~/.idapt/config.json` (`theme` field), so the choice survives
across `idapt tui` runs. `NO_COLOR=1` (env) or `--no-color` (flag) trumps
everything and renders monochrome.

## Mouse toolbar

A one-line button strip sits just above the status bar:

```
[ Menu ] [ New ] [ Help ] [ Model ] [ Agent ] [ Project ] [ Quit ]
```

`[ Menu ]` (primary-styled) opens the **command palette** — a modal
list of every slash command including `/theme`, which then opens its
own theme picker for one-keystroke flips. Arrow keys navigate, Enter
dispatches, Esc dismisses, **clicking outside the dialog closes it**.
While streaming, the bar collapses to `[ Stop ] [ Menu ] [ Help ]
[ Quit ]`.

Clicks dispatch the same code path as the matching slash command, so
keyboard + mouse stay in lockstep. Mouse-wheel scroll over the
transcript pages it.

The toolbar is painted from the moment the TUI receives its first
`WindowSizeMsg`, so the bar is visible on the very first frame —
no longer the "buttons invisible until I click" symptom that came
from refreshing the toolbar inside View() (where mutations got
discarded with each render).

## Modal dialogs

When a picker opens, the chat surface **stays visible but dimmed**
behind the dialog — the user keeps the conversation in peripheral
vision while focused on the modal. Dismiss in any of:

* Press Enter (selects the focused row).
* Press Esc.
* Click on a row (selects).
* Click anywhere on the dimmed backdrop (closes).
* Click the `[ Esc · Close ]` chip in the dialog footer.

The footer carries two button chips — `[ Enter · Select ]` (primary)
and `[ Esc · Close ]` — both are clickable in addition to their
keyboard equivalents. Buttons render with rounded borders to read as
real interactive elements rather than text hints.

## Composer

The input is **edge-to-edge** with only top and bottom horizontal
rules (no side borders) so it visually integrates with the chat
surface rather than appearing as a separate boxed input. The prompt
symbol switches between `>` (neutral) and `▸` (a registered slash
command is recognized at the cursor), both styled in the theme's
primary color when a command is detected.

Single-line by default; expands up to 8 rows as the user adds
newlines / wraps, shrinks back to one row when the buffer empties.

## Errors

`AppendError` renders as a loud red row with a `✖` glyph and the
"Error" label so it can't be missed in a long transcript:

```
✖ Error
  ✖ not authenticated — set IDAPT_API_KEY or run `idapt config set api-key <token>`
```

Multi-line error bodies keep the indent on every continuation line.

## Newlines

Submit is bound to `Enter`. To insert a newline use either:

| Key         | Notes                                                                    |
|-------------|--------------------------------------------------------------------------|
| `Alt+Enter` | **Primary.** Bubbletea v1 parses this cleanly across every terminal.     |
| `Ctrl+J`    | **Fallback.** LF (0x0a) is always distinct from Enter byte-wise.         |

Shift+Enter / Ctrl+Enter are **not** wired — bubbletea v1 has no parser
for the modifyOtherKeys / Kitty keyboard protocols those keys rely on,
so terminals either collapse them to plain Enter (legacy mode) or emit
unparsable escape sequences. We filter the latter at the Model layer
so they never inject literal `[27;2;13~`-style garbage into the
composer, but we don't pretend to support them either.

The composer is **single-line by default** and grows up to 8 rows as you
add newlines / wrap; it shrinks back to one row when the buffer empties.

When typing a slash command, newlines hide the autocomplete menu so
the user can compose multi-line drafts that happen to start with `/`
without accidentally triggering verb completion.

## One-shot mode

```bash
idapt -p "explain this regex"          # root-flag shortcut
idapt chat ask "what is 2+2?" --stream # canonical subcommand
echo "x" | idapt -p                    # pipe stdin as prompt
idapt -p "..." --file foo.ts           # attach a file
idapt chat send <chat-id> "msg" --stream  # stream an existing chat
```

Streaming is enabled by default when stdout is a TTY. Pipe-to-file or
`--no-stream` falls back to sync POST.

Exit codes:

| Code | Meaning            |
|------|--------------------|
| 0    | Success            |
| 2    | Auth failure       |
| 3    | Network error      |
| 4    | Spending cap hit   |
| 5    | Rate-limited       |
| 1/10 | Other / generic    |

## Persisted state

`~/.idapt/config.json` is augmented with:

* `lastAgentId` — the agent the TUI used most recently.
* `lastModelId` — the model the TUI used most recently.
* `lastChatId` — the chat that was open at quit; reopens on next boot.

These fields are written on graceful quit and after each completed stream.
A file lock + atomic rename guarantees concurrent `idapt` processes don't
corrupt the JSON. See `internal/cliconfig/Cliconfig.md` for the schema.

## Authentication

Identical to the rest of the CLI:

1. `--api-key <token>` flag (highest priority).
2. `IDAPT_API_KEY` environment variable.
3. `~/.idapt/credentials.json` (written by `idapt config set api-key …`).

Legacy `mk_*` tokens are ignored (machine-keys; see commit history).

## Architecture

The TUI is a pure presentation layer over `internal/api/`. It never opens
HTTP connections of its own — every byte goes through the same Go client
that backs `idapt chat send`, `idapt chat ask`, and every other Cobra
verb. See `internal/tui/Tui.md` for the module map; the rule is enforced
by `scripts/check-tui-no-http.sh`.

| Concern            | Implementation                                          |
|--------------------|---------------------------------------------------------|
| Event loop         | Bubble Tea v1 (Elm-style Model / Update / View).        |
| Layout             | Lipgloss adaptive styles + JoinVertical / Place.        |
| Widgets            | bubbles/textarea + viewport + list + textinput.         |
| Markdown rendering | Glamour with synthetic-fence handling for streams.      |
| Slash commands     | `internal/tui/commands` parser + registry.              |
| SSE bridge         | `internal/tui/stream` wraps `api.StreamSSE`.            |
| Reconnect          | Exponential backoff 500ms→1s→2s→5s, max 5 retries.      |
| Clipboard          | OSC52 escape (iTerm2, kitty, WezTerm, Alacritty, etc.). |

## Terminal compatibility

| Terminal           | Status   | Notes                              |
|--------------------|----------|------------------------------------|
| iTerm2             | full     | OSC52 + truecolor                  |
| kitty              | full     | OSC52 + truecolor                  |
| WezTerm            | full     |                                    |
| Alacritty          | full     |                                    |
| Windows Terminal   | full     | recommended for Windows            |
| macOS Terminal.app | partial  | OSC52 not supported (no /copy)     |
| GNOME Terminal     | partial  | OSC52 default-off                  |
| tmux               | full     | needs `set -g set-clipboard on`    |
| Linux text console | degraded | ASCII fallback; no truecolor       |
| Windows cmd.exe    | unsupported | use Windows Terminal instead    |

## Troubleshooting

* **TUI boots when I didn't want it** — set `IDAPT_NO_TUI=1`.
* **Composer text vanished after disconnect** — it shouldn't; if it does,
  file an issue. The reconnect loop preserves it intentionally.
* **Stuck in alt-screen after a crash** — `reset` or `tput rmcup` in your
  shell restores normal output.
* **Markdown looks pixelated** — your terminal doesn't support truecolor;
  set `NO_COLOR=1` or use a modern terminal.
