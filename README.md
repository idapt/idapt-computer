<p align="center">
  <img src="https://idapt.ai/images/logo/logo.png" alt="idapt" width="120" />
</p>

<h1 align="center">idapt CLI</h1>

<p align="center">
  Your AI workspace, from the terminal.<br />
  The per-computer daemon, Drive FUSE mount, and chat TUI — plus the
  <code>idapt-client</code> resource grammar (agents, files, 200+ models).
</p>

<p align="center">
  <a href="https://idapt.ai/cli"><strong>Landing Page</strong></a> &middot;
  <a href="https://idapt.ai/help/cli-overview"><strong>Documentation</strong></a> &middot;
  <a href="https://github.com/idapt/idapt-cli/releases"><strong>Releases</strong></a>
</p>

---

## Install

```bash
curl -fsSL https://idapt.ai/cli/install | bash
```

The installer drops the binary into a per-user location:

- **Linux + macOS:** `~/.local/bin/idapt` — rootless, no sudo, XDG-aligned.
  Modern distros put `~/.local/bin` on `$PATH` automatically; the installer
  prints a one-line shell-rc snippet otherwise.
- **System-wide install** (shared boxes, Docker images, CI):
  `INSTALL_DIR=/usr/local/bin curl -fsSL https://idapt.ai/cli/install | bash`
  — the only path that prompts for sudo.
- **Existing install detected on `$PATH`** → upgrade in place at that location.

Per-user runtime state follows platform conventions:

| | Config (credentials, settings) | Cache (model list, FF cache, FUSE blobs) | Persistent data (local-inference) |
|---|---|---|---|
| **Linux** | `$XDG_CONFIG_HOME/idapt/` (`~/.config/idapt/`) | `$XDG_CACHE_HOME/idapt/` (`~/.cache/idapt/`) | `$XDG_DATA_HOME/idapt/` (`~/.local/share/idapt/`) |
| **macOS** | `~/Library/Application Support/idapt/` | `~/Library/Caches/idapt/` | `~/Library/Application Support/idapt/` |
| **Windows** | `%AppData%\idapt\` | `%LocalAppData%\idapt\` | `%LocalAppData%\idapt\` |

Or download directly from [GitHub Releases](https://github.com/idapt/idapt-cli/releases).

## Interactive TUI

Running `idapt` with no subcommand in a terminal launches an interactive
**Bubble Tea TUI** for conversational chat:

```bash
idapt                         # auto-boots when stdin+stdout are TTYs
idapt tui                     # explicit alias
```

Pickers for workspace / model / agent, slash commands (`/help`, `/new`,
`/file`, …), streaming markdown with cancellation, OSC52 clipboard.
See [TUI.md](TUI.md) for the full reference.

For non-interactive scripts use `idapt -p` (shortcut) or the canonical
subcommand `idapt chat ask`:

```bash
idapt -p "explain this regex"
echo "x" | idapt -p
idapt chat ask "..."
```

## API surface

The CLI talks to idapt's **public v1 REST API** (`/api/v1/*`) for every
user-resource action — chats, agents, workspaces, files, computers, secrets,
sharing, store, audio, web, code execution. External developers can hit
the same endpoints. See [Public API docs](https://idapt.ai/api/v1/docs)
(Scalar) for the full reference.

A handful of CLI-internal bootstrap concerns (email sign-in, daemon pair
JWKS, FF visibility cache, auto-update binary fetch, FUSE adapter) stay
on the internal `/api/*` paths because they are first-party plumbing,
not part of the stable contract.

The Go binary handles the per-computer daemon and the native, on-machine
surfaces. The **resource grammar** (`idapt <resource> <verb>` — agents, chats,
Drive ops, models, web, computers, …) is the npm
[`@idapt/agent-tools` `idapt-client`](../../packages/agent-tools/README.md), the
one client that also backs the in-chat agent dispatcher and MCP — so the CLI, the
agent, the SKILL, and MCP all speak ONE grammar over the public v1 API.

## Quick Start

```bash
# 1. Authenticate + bring this computer online (native daemon)
idapt auth login --api-key uk_...
idapt up                          # install + authorize + start the daemon

# 2. Work on this machine
idapt chat ask "Summarize the latest report"   # interactive TUI (FF-gated)
idapt drive mount ~/idapt                        # FUSE-mount your Drive
idapt expose 3000 --public                       # public tunnel to a local port

# 3. The resource CRUD lives in the npm client (same grammar as the in-app agent)
npm i -g @idapt/agent-tools
idapt-client agent list -o json
```

## Features (Go CLI — native / daemon only)

- **Per-computer daemon** — heartbeat, command channel, tunnel client, firewall
- **Drive FUSE mount** — `idapt drive mount` + cloud file sync
- **Interactive TUI** — `idapt chat ask` (Bubble Tea, FF-gated)
- **Networking** — `expose` / `unexpose`, `tunnel`, `ssh`, `firewall`
- **Local Inference** — install/start/pull/prompt private Ollama models
- **Lifecycle** — `up` / `down` / `logout`, `service`, `update`, `desktop` hooks

The resource grammar — 200+ models, agents & chat, Drive ops (read/write/grep/
glob/search), cloud code run, image/audio, web search, computer apps, triggers &
hooks, subagents, subscriptions — moved to the npm `idapt-client`.

## Usage

```bash
# Native (this binary)
idapt auth login --api-key uk_...
idapt up
idapt drive mount ~/idapt
idapt chat ask "Hello"

# Resource CRUD (the npm idapt-client — identical grammar to the in-app agent)
idapt-client agent create --name "My Agent" --system-prompt "You are helpful"
idapt-client drive list -o json
echo '{"name":"agent","icon":"emoji/🤖"}' | idapt-client agent create --json -
```

## Command Groups (native / daemon)

| Group | Commands | Description |
|-------|----------|-------------|
| `auth` | login, logout, status | Authentication |
| `config` | set, get, list | CLI configuration (per-OS user config dir) |
| `chat` | ask | Interactive TUI (Bubble Tea, FF-gated) |
| `drive` | mount, unmount | FUSE-mount your Drive + cloud sync |
| `local-inference` | status, install, start, stop, logs, models, pull, remove, ask, setup | Private daemon-backed Ollama runtime + provider endpoint setup |
| `up` / `down` / `logout` | | Daemon lifecycle (Tailscale-style verbs) |
| `service` | up, down, restart, status, logs, uninstall | Manage the local daemon |
| `serve` | | Foreground daemon (used by the autostart unit; debug via `--verbose`) |
| `pair` | | Pair this computer with idapt (daemon bootstrap) |
| `expose` / `unexpose` | | Expose / close a local port as a public tunnel |
| `tunnel` | list, stop | Inspect this computer's tunnels |
| `ssh` | | SSH into a computer through the idapt tunnel |
| `firewall` | list, add, remove | Daemon-side firewall management |
| `open` | | Open the idapt web app (or a resource) in your browser |
| `desktop` | register, purge, archive, … | Lifecycle hooks invoked by the desktop app |
| `instructions` | | Show a command's playbook (when/why/anti-patterns) |
| `completion` | | Generate / install shell autocompletion scripts |
| `version` | | Print CLI version |
| `update` | | Self-update binary after SHA256 + Ed25519 signature verification |
| `uninstall` | | Remove idapt from this machine (`--purge` also wipes config/cache/data) |

The **resource grammar** (`workspace` / `agent` / `chat` CRUD / `drive` file ops /
`secret` / `script` / `model` / `exec` / `web` / `media` / `computer` (incl.
`computer app`) / `share` / `hub` / `subagent` / `trigger` / `hook` / `settings` /
`profile` / `api-key` / `subscription` / `notification`) is the npm
**`idapt-client`** — same grammar as the in-app agent + MCP, over the public v1
API. See `packages/agent-tools/README.md`; computer-app semantics + Compose
validation are documented in
`backend/packages/computers/src/internal/computers/Computers.md`.

## Global Flags

```
--api-key string   API key for authentication (or IDAPT_API_KEY env)
--api-url string   API base URL (default https://idapt.ai)
--workspace string   Default workspace slug (or IDAPT_PROJECT env)
-o, --output       Output format: table|json|jsonl|quiet
--verbose          Show request/response details
--confirm          Skip confirmation prompts for destructive ops
--no-color         Disable color output
```

## Input/Output

**Input modes** (for the npm `idapt-client` create/edit commands):
- Named flags: `--name "My Agent" --icon "emoji/🤖"`
- JSON flag: `--json '{"name":"test","systemPrompt":"..."}'`
- JSON from stdin: `echo '{}' | idapt-client agent create --json -`
- File flags: `--system-prompt-file ./prompt.md`

**Output formats** (`-o` flag):
- `table` — human-readable columns (default for TTY)
- `json` — computer-readable JSON (default when piped)
- `jsonl` — one JSON object per line
- `quiet` — IDs only

## Architecture

```
services/idapt/
├── cmd/                    # Cobra UX layer (registration, flags, help, thin handlers)
│   ├── root.go             # Global flags, PersistentPreRunE, command wiring
│   ├── auth.go             # auth login/logout/status
│   ├── agent.go            # agent CRUD
│   ├── computer.go          # computer wiring
│   ├── computer_core.go     # computer lifecycle
│   ├── computer_file.go     # computer remote files
│   ├── computer_tmux.go     # computer tmux sessions
│   └── ...                 # 25+ command groups total
├── internal/
│   ├── api/                # REST API HTTP client (auth, retry, SSE, upload/download)
│   ├── cliconfig/          # CLI config (per-OS user config dir, see idaptpaths)
│   ├── credential/         # Credential storage (per-OS user config dir)
│   ├── idaptpaths/         # Single source of truth for ConfigDir / CacheDir / DataDir (XDG-aligned)
│   ├── output/             # Output formatters (table, JSON, JSONL, quiet)
│   ├── input/              # --json and --*-file flag parsing
│   ├── resolve/            # Resource name → ID resolution with caching
│   ├── cmdutil/            # Factory (DI), global flags, exit codes, confirm
│   ├── httpclient/         # Version header transport (User-Agent, X-Idapt-Version)
│   ├── commands/           # Daemon SSE+POST command executor, not Cobra commands
│   ├── auth/               # Daemon JWT/HMAC/API key validation
│   ├── config/             # Daemon config (/etc/idapt/config.json)
│   ├── firewall/           # Daemon iptables management
│   ├── tunnelclient/       # Outbound tunnel data plane (to the tunnel-proxy)
│   └── ...                 # Other daemon packages
```

## API Version Handling

Every request includes `User-Agent: idapt-cli/{version}` and
`X-Idapt-Version: {api-version}` via `internal/httpclient`. Authenticated API
requests send the configured key as `Authorization: Bearer ...`; this is the
public v1 contract, while internal `/api/*` routes accept the same bearer
credential for first-party CLI plumbing. The CLI ignores unknown response
fields and handles missing optional fields for forward/backward compatibility.
See root `CLI.md` and `API_Versioning.md`.

## Versioning + self-update

The CLI follows strict [SemVer](https://semver.org/) — releases are tagged
`cli-vMAJOR.MINOR.PATCH` (e.g. `cli-v1.4.2`). Dev builds carry a
pre-release tail (`cli-v1.4.2-dev.<short-sha>`) which per semver sorts
below the corresponding release, so a dev build always self-updates to
the next real release.

`idapt update` polls the staged-rollout-aware manifest at
`https://idapt.ai/api/cli/manifest/{platform}/{arch}` every 6h, verifies
the SHA-256 + Ed25519 signature of the downloaded binary against the
embedded `release-pubkey.pem` trust root, and atomically swaps the
binary in place. The version comparator
(`internal/update.CompareVersions`) handles the semver rules
(major/minor/patch as integers, pre-release < release).

A new release is cut by clicking **▶ cli:release** on a `main`-branch
GitLab pipeline. That job runs the shared `cut-release-tag.sh` helper
(same script powers `desktop:release` and `sdk:release`), derives the
next semver from Conventional Commits since the last `cli-v*` tag, and
pushes the new tag back via a `write_repository` token
(`IDAPT_RELEASE_PUSH_TOKEN`, falling back to the deploy ledger's
`IDAPT_GITOPS_PUSH_TOKEN` — `CI_JOB_TOKEN` cannot push on gitlab.com; see
`.gitlab/ci/CI_CD_Variables_Setup.md` → "Release Tag Push"). Because the
button computes the version, the tag is always a valid `cli-vX.Y.Z`.

Only a **strict** `cli-vX.Y.Z` tag triggers a release — the pipeline rules
pin `^cli-v[0-9]+\.[0-9]+\.[0-9]+$`, so a hand-created malformed/typo tag
matches no job and publishes nothing. The tag pipeline then builds, signs,
uploads to R2, and syncs to github.com/idapt/idapt-cli. See
`.gitlab/ci/cli.yml` for the full job graph.

## Documentation

- [CLI Landing Page](https://idapt.ai/cli) — installation, features, and quick start
- [Help Center](https://idapt.ai/help/cli-overview) — full documentation with guides
- [Command Reference](https://idapt.ai/help/cli-commands) — all 24 command groups
- [Automation Guide](https://idapt.ai/help/cli-automation) — scripting and CI/CD

## License

MIT &copy; 2026 idapt — see [LICENSE](LICENSE)
