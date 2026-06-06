<p align="center">
  <img src="https://idapt.ai/images/logo/logo.png" alt="idapt" width="120" />
</p>

<h1 align="center">idapt CLI</h1>

<p align="center">
  Your AI workspace, from the terminal.<br />
  Manage workspaces, agents, files, chats, and 200+ AI models.
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
idapt chat ask "..." --stream
idapt chat send <chat-id> "follow-up" --stream
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

## Quick Start

```bash
# 1. Authenticate
idapt auth login --api-key uk_...

# 2. Explore your workspace
idapt workspace list -o table
idapt agent list --workspace my-workspace

# 3. Start working
idapt chat send my-chat --message "Summarize the latest report"
idapt drive upload ./data.csv --workspace my-workspace
```

## Features

- **200+ AI Models** — access every model from your terminal
- **Agents & Chat** — create agents, send messages, export conversations
- **Drive** — upload, download, grep, glob, semantic search, cloud sync
- **Cloud Code Run** — sandboxed Python and Node.js in the cloud
- **Image & Audio** — generate images and transcribe audio
- **Web Search** — search the web and fetch pages
- **Local Inference** — install/start/pull/prompt private Ollama models through a paired daemon
- **Computer Apps** — Docker-compatible apps on a paired computer via
  `idapt computer app`
- **Triggers & Hooks** — automate agent runs and manage condition-driven
  per-turn instructions
- **Subagent** — orchestrate agent conversations
- **Subscriptions** — manage billing and plans from the CLI

## Usage

Interact with idapt from any terminal — manage workspaces, agents, Drive (cloud files), chats, and more.

```bash
# Authenticate
idapt auth login --api-key uk_...
idapt auth status

# Manage resources
idapt workspace list -o json
idapt agent create --name "My Agent" --system-prompt "You are helpful"
idapt drive upload ./data.csv
idapt chat send my-chat --message "Hello"

# JSON input for agents/automation
echo '{"name":"agent","icon":"emoji/🤖"}' | idapt agent create --json -
```

## Command Groups

| Group | Commands | Description |
|-------|----------|-------------|
| `auth` | login, logout, status | Authentication |
| `config` | set, get, list | CLI configuration (per-OS user config dir) |
| `workspace` | list, create, get, edit, delete, invitation, member | Workspace management (slug-based invitations) |
| `agent` | list, create, get, edit, delete | Agent management |
| `chat` | list, create, get, edit, delete, send, reprompt, messages, runs, cost, archive, unarchive, restore, permanent-delete, stop, export | Chat & messaging |
| `drive` | list, read, write, create, edit, delete, rename, move, mkdir, grep, glob, search, upload, download, mount, unmount | Drive file operations |
| `secret` | list, create, get, edit, delete | Workspace-scoped credential storage |
| `script` | list, create, get, edit, delete, run, runs, output, interrupt | Code script files and cloud runs |
| `model` | list, search | Model browsing |
| `local-inference` | status, install, start, stop, logs, models, pull, remove, ask, setup | Private daemon-backed Ollama runtime and provider endpoint setup |
| `exec` | file | Run code files in the cloud sandbox |
| `web` | search, fetch | Web search + URL fetch (with SSRF guard) |
| `media` | generate, list-models, transcribe, speak, list-voices, list-tts-models | Image + audio generation |
| `computer` | list, create, get, edit, delete, archive, unarchive, start, stop, test, exec, tmux, file, dir, fw, port, user, env-var, app | Daemon-backed computer management (commands, files, users, env, firewall, Docker-backed apps) |
| `share` | list, grantees, create, update, delete | Resource sharing (unified `/v1/shares`) |
| `hub` | search, install, submit, submissions | Hub marketplace (FF42-gated) |
| `subagent` | chat create, chat list, chat edit, message send, message list, message get | Subagent orchestration (aliased over `/v1/chats`) |
| `trigger` | list, create, get, edit, delete, rotate-secret, runs, fire | Cron/webhook automation |
| `hook` | list, create, get, update, toggle, override, delete | Condition-driven instruction hooks |
| `settings` | get, set | Account preferences |
| `profile` | get, edit | Profile (name + slug) |
| `api-key` | list, create, delete | API key management (cookie session only) |
| `subscription` | status, usage | Billing + usage |
| `notification` | list, read | Notifications |
| `firewall` | list, add, remove | Daemon-side firewall management |
| `pair` | | Pair this computer with idapt (daemon bootstrap) |
| `expose` / `unexpose` | | Expose / close a local port as a public tunnel |
| `tunnel` | list, stop | Inspect this computer's tunnels |
| `version` | | Print CLI version |
| `update` | | Self-update binary after SHA256 + Ed25519 signature verification |
| `serve` | | Foreground daemon (used by the autostart unit; debug via `--verbose`) |
| `service` | up, down, restart, status, logs, uninstall | Manage the local daemon — Tailscale-style verbs; idempotent `up` installs the autostart unit on first run |
| `uninstall` | | Remove idapt from this machine (binary + autostart). `--purge` also wipes config/cache/data. Interactive confirm by default. |

### Computer Apps

```bash
idapt computer app status <computer>
idapt computer app setup <computer>
idapt computer app list <computer>
idapt computer app external <computer>
idapt computer app create <computer> --name web --image nginx:alpine
idapt computer app run <computer> ./services/web --dockerfile Dockerfile
idapt computer app compose-up <computer> compose.yaml --project-directory deploy --accept-policy-warnings
idapt computer app logs <computer> <app>
idapt computer app exec <computer> <app> -- npm test
idapt computer app shell <computer> <app> -- npm test
idapt computer app ports <computer> <app>
idapt computer app expose <computer> <app> 3000 --public
idapt computer app unexpose <computer> <app> 3000
idapt computer app start|stop|restart|reset|delete <computer> <app>
```

Computer Apps are FF63-gated and use the same public v1 API contract as the
rest of the user-facing CLI. The daemon uses the host's Docker-compatible
runtime, adds Idapt labels as metadata, and validates Compose files to block
privileged containers, host namespaces, device mounts, added capabilities,
Docker socket mounts, absolute host binds, and common credential directory
mounts. It does not edit checked-in Dockerfiles or Compose files. External
Docker containers can be listed read-only with `idapt computer app external`.

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

**Input modes** (for create/edit commands):
- Named flags: `--name "My Agent" --icon "emoji/🤖"`
- JSON flag: `--json '{"name":"test","systemPrompt":"..."}'`
- JSON from stdin: `echo '{}' | idapt agent create --json -`
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
