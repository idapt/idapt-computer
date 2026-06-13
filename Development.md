# Local CLI development

Internal-only notes on iterating on the idapt CLI from this dev
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
npm run cli:run -- version
npm run cli:run -- drive mount ~/idapt
npm run cli:run -- auth status
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
idapt --api-url https://idapt.ai auth status
```

You also need a user credential. `idapt auth login` is the one-step path: it
signs in and exchanges your session for a long-lived, revocable `uk_` API key,
then stores THAT (never your password) at the per-OS user config dir
(`$XDG_CONFIG_HOME/idapt/credentials.json` on Linux, mode 0600):

```bash
# Interactive — prompts for the password with echo disabled:
idapt auth login --email dev@idapt.ai
# Or fully scripted, password piped (never in argv / shell history):
printf '%s' 'TestPassword123!@#' | idapt auth login --email dev@idapt.ai --password-stdin
```

Any signed-in account can mint a key — the **free** tier included (`dev@` works
out of the box); only anonymous, not-signed-in sessions are refused. To skip
email+password, paste a key created in the web UI (Settings → API Keys → Create)
— also via stdin or env to keep it out of argv:

```bash
printf '%s' "$KEY" | idapt auth login --api-key-stdin   # persisted to credentials.json
export IDAPT_API_KEY=uk_dev_...                          # or one-off via env
```

Never pass `--password`/`--api-key`/`--token` with an inline value (they land in
shell history + the process list — the CLI warns when you do). Prefer the
`--*-stdin` flags, the interactive prompt, or the `IDAPT_API_KEY` / `IDAPT_TOKEN`
env vars.

A future change will pre-seed a deterministic dev API key into the snapshot
generator + `.env.cli` so even this step drops to zero; for now the above is the
dev loop.

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
`completion / config` menu without any extra setup.

Dynamic completion sources hit the live API with a 2s hard deadline and a
30s in-process cache; if the server is slow or offline the shell falls
back to filename completion (no hang). The source files:

* `services/idapt/cmd/completion.go` — the user-facing `completion` /
  `completion install` command tree.
* `services/idapt/cmd/completion_helpers.go` — dynamic completion
  sources for workspaces, plus the in-process cache.
* Per-verb wiring lives next to the verb itself — search for
  `ValidArgsFunction:` and `RegisterFlagCompletionFunc(`.

## Running tests

| Command | Scope | Speed |
|---|---|---|
| `npm run cli:test` | All Go unit tests under `services/idapt/...` | seconds |
| `npm run cli:vet` | `go vet ./...` | <1s |

The full prod integration suite (`npm run test:cli:integration`) still spins
a k3d cluster, installs the local platform, builds the production app image,
and then runs the Go integration package. Use it before merging to validate
against a real backend, not for daily iteration. CI gives this job the same
150-minute cold-start budget as the integration shards because fresh runners
can spend most of the run pulling controller images before the app image build
starts.

## Regenerating the agent SKILL.md

`services/idapt/SKILL.md` is **generated**, not hand-edited. It is the canonical
CLI skill — served at `GET /cli/skill`, wrapped into the in-app system library
as `instructions/idapt-cli.md`, and mirrored to `.claude/skills/idapt-cli/SKILL.md`
for this repo's Claude Code harness.

```bash
npm run cli:skill:generate   # rewrites both SKILL.md copies
```

The command-surface grid (the `▸ verb — hint` lines) is rendered from the
dispatcher's `ACTION_REGISTRY` through the shared
`backend/products/chat/src/internal/tools/dispatcher/render-command-surface.ts`, so it stays in parity
with what the in-app idapt agent sees. Edit the prose, frontmatter, or the
CLI-only resource list in `scripts/generate-cli-skill.ts` — never the `SKILL.md`
files directly. `scripts/generate-cli-skill.test.ts` fails CI if a committed
copy drifts from the generator output.

## Common iteration loops

**Command behavior tweak:**
```bash
npm run cli:install && idapt <command>
```

**cmd-package change:**
```bash
cd services/idapt && go test ./cmd/...
```

**Before committing:**
```bash
npm run cli:vet && npm run cli:test
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
