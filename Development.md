# Local CLI development

Internal-only notes on iterating on the idapt CLI / TUI from this dev
container. Not for public consumption — the user-facing entry point stays
[README.md](./README.md).

## One-liner: build + install

```bash
npm run cli:install
```

Builds `services/idapt` and drops the binary at `/usr/local/bin/idapt` so you
can run `idapt` from any shell in the container. Idempotent — re-run after
every change.

The other variant `npm run cli:build` writes to `bin/idapt` at the repo root
instead (handy if you don't want to shadow whatever the dev container ships).
That path is gitignored.

## Run from source without installing

```bash
npm run cli:run -- tui
npm run cli:run -- chat ask "explain this regex"
npm run cli:run -- -p "hello"
```

`cli:run` does `go run .` so each invocation rebuilds incrementally. Slightly
slower than `cli:install` but you don't need to remember to rebuild.

## Pointing at a local backend

The CLI talks to whatever `IDAPT_API_URL` resolves to (precedence: `--api-url`
flag → `IDAPT_API_URL` env → CLI config `apiUrl` (under the per-OS user config
dir — `$XDG_CONFIG_HOME/idapt/config.json` on Linux, see
`internal/idaptpaths/`) → compiled-in default `https://idapt.ai`).

**In this dev container**, `.env.cli` already exports
`IDAPT_API_URL=http://localhost:3000` into every shell, so the CLI hits the
Tilt-managed local cluster by default. No setup needed beyond `npm run dev`
being up. If you want a one-off prod invocation in the dev container, pass
the flag explicitly:

```bash
idapt --api-url https://idapt.ai chat send <chat-id> "..."
```

You still need an API key for the local cluster — mint one via the web app
running at `http://localhost:3000` (sign in as `dev@idapt.ai` /
`TestPassword123!@#`, Settings → API Keys → Create), then either:

```bash
# One-off
export IDAPT_API_KEY=uk_dev_...
idapt tui

# Persisted across shells (writes to the per-OS user config dir —
# `$XDG_CONFIG_HOME/idapt/credentials.json` on Linux)
idapt auth login --api-key uk_dev_...
```

A future change will pre-seed a deterministic dev API key into the snapshot
generator + `.env.cli` so the key step also drops to zero, but that's
outside the CLI scope; for now the 30-second manual step is the dev loop.

## TUI-only smoke (no backend needed)

If you just want to see the TUI render — header, composer, slash commands,
pickers showing their error states, /help overlay — point at a dead endpoint:

```bash
IDAPT_API_KEY=dummy IDAPT_API_URL=http://127.0.0.1:1 \
  npm run cli:run -- tui
```

The shell of the experience works (composer, /help, /clear, /quit, picker
modal opening). Anything that hits the network surfaces an error inline.

## Shell completion

The CLI ships a Cobra-generated tab-completion driver for bash, zsh, fish,
and PowerShell, plus a top-level `idapt completion install` verb that
prints the right one-liner for the user's current shell.

```bash
# Detect $SHELL and print install instructions for that shell.
idapt completion install

# Or force a specific shell.
idapt completion install bash
idapt completion install zsh

# Each per-shell generator is also exposed directly (cobra defaults):
idapt completion bash > /etc/bash_completion.d/idapt
```

In **this dev container** the bashrc already auto-sources the completion
driver every time you start a shell (see `.devcontainer/Dockerfile`), so
after `npm run cli:install` you can `idapt c<TAB>` and get the
`chat / completion / config / computer` menu without any extra setup.

Dynamic completion sources hit the live API with a 2s hard deadline and a
30s in-process cache; if the server is slow or offline the shell falls
back to filename completion (no hang). The source files:

* `services/idapt/cmd/completion.go` — the user-facing `completion` /
  `completion install` command tree.
* `services/idapt/cmd/completion_helpers.go` — dynamic completion
  sources for chats, agents, workspaces, models, triggers, plus the
  in-process cache.
* Per-verb wiring lives next to the verb itself (`chat.go`, `agent.go`,
  `workspace.go`, `trigger.go`, `model.go`) — search for
  `ValidArgsFunction:` and `RegisterFlagCompletionFunc(`.

## Running tests

| Command | Scope | Speed |
|---|---|---|
| `npm run cli:test` | All Go unit + model tests under `services/idapt/...` | seconds |
| `npm run cli:test:tui` | Just the TUI tree + its prerequisites (api, cliconfig) | <1s cached |
| `npm run cli:test:tui:integration` | TUI + httptest server (build-tag gated) | ~1-2s |
| `npm run cli:test:tui:e2e` | Real binary spawned in PTY + vt10x screen reads | ~5s |
| `npm run cli:vet` | `go vet ./...` | <1s |
| `npm run cli:lint:no-http` | Enforces no `net/http` imports under `internal/tui/` | instant |
| `npm run cli:lint:docs` | Enforces every TUI folder has a `.md` and the cross-refs exist | instant |

The full prod integration suite (`npm run test:cli:integration`) still spins
a k3d cluster, installs the local platform, builds the production app image,
and then runs the Go integration package. Use it before merging to validate
against a real backend, not for daily iteration. CI gives this job the same
150-minute cold-start budget as the integration shards because fresh runners
can spend most of the run pulling controller images before the app image build
starts.

## Common iteration loops

**TUI behavior tweak:**
```bash
npm run cli:install && idapt tui
```

**Slash-command / parser change:**
```bash
npm run cli:test:tui
```

**SSE / streaming change:**
```bash
npm run cli:test:tui && npm run cli:test:tui:integration
```

**`chat ask` / `-p` / `chat send --stream` change:**
```bash
cd services/idapt && go test ./cmd/... -run "TestChat"
```

**Before committing:**
```bash
npm run cli:vet && npm run cli:test:tui && \
  npm run cli:test:tui:integration && \
  npm run cli:test:tui:e2e && \
  npm run cli:lint:no-http && npm run cli:lint:docs
```

## Notes

* **Go 1.25** is installed system-wide by the dev container Dockerfile
  (`.devcontainer/Dockerfile`, `GO_VERSION=1.25.0`). It lives at
  `/usr/local/go/bin/go` with a symlink at `/usr/local/bin/go`, and
  `/etc/profile.d/go.sh` exports `GOPATH=/root/go` + the right `PATH` for
  every shell, including the non-login subshells npm spawns. `go version`
  should always report `go1.25.x` in this container.
* If you rebuild the container locally with an older base or notice
  `invalid go version '1.25.0': must match format 1.23`, that means
  Debian's `golang-go` (1.19) is shadowing the upstream install. The
  Dockerfile removes it; manually purge with `apt-get remove -y golang-go
  golang-1.19-go` to recover without a rebuild.
* The TUI uses `bubbletea` + `lipgloss` + `glamour` + `bubbles`. Don't add new
  Charm deps without checking the `lipgloss` version constraint (`glamour`
  pins a specific pre-release).
* PTY tests force `NO_COLOR=1` + `TERM=dumb` to skip termenv's 5-second OSC
  11 background-color query — without that, every E2E test wall-clock-blocks
  on the query timeout.
* The CLI never auto-boots the TUI when `IDAPT_NO_TUI=1` is set. Useful for
  wrapper scripts that need to run idapt verbs in a tty.
