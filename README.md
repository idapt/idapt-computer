<p align="center">
  <img src="https://idapt.app/images/logo/logo.png" alt="idapt" width="120" />
</p>

<h1 align="center">idapt computer</h1>

<p align="center">
  Connect any computer to your <a href="https://idapt.app">idapt</a> AI workspace.<br />
  Register a machine, mount your Drive, expose local ports, and run private
  on-device models — all from the terminal.
</p>

<p align="center">
  <a href="https://idapt.app/cli"><strong>Overview</strong></a> &middot;
  <a href="https://idapt.app/help/cli-overview"><strong>Documentation</strong></a> &middot;
  <a href="https://github.com/idapt/idapt-computer/releases"><strong>Releases</strong></a>
</p>

---

## Install

Linux:

```bash
curl -fsSL https://idapt.app/computer/install | bash
```

Windows PowerShell:

```powershell
$env:IDAPT_TOKEN = "pt_..."
iwr -useb "https://idapt.app/api/computer/install.ps1" | iex
```

The installer drops a single static binary into a per-user location (no sudo on
Linux — it uses `~/.local/bin`; Windows uses `%LOCALAPPDATA%\Programs\Idapt\cli`).
For a system-wide Linux install on shared boxes, Docker images, or CI:

```bash
INSTALL_DIR=/usr/local/bin curl -fsSL https://idapt.app/computer/install | bash
```

You can also grab a binary directly from
[GitHub Releases](https://github.com/idapt/idapt-computer/releases).

## Quick start

```bash
idapt auth login --api-key uk_...   # authenticate (Settings → API keys in the app)
idapt up                            # bring this computer online (installs + starts the daemon)

idapt drive mount ~/idapt           # FUSE-mount your idapt Drive
idapt expose 3000 --public          # share a local port as a public URL
idapt computer terminal my-computer # open an interactive terminal (daemon PTY)
```

## What it does

- **Per-computer daemon** — registers this machine to your workspace; heartbeat,
  command channel, and local command policy.
- **Drive** — `idapt drive mount` for a live FUSE mount of your files + cloud sync.
- **Networking** — `expose` / `unexpose` public tunnels, `ssh`, and `tunnel`.
- **Local inference** — install and run private [Ollama](https://ollama.com)
  models on this machine and use them from idapt (`idapt local-inference …`).
- **Lifecycle** — `up` / `down`, `service` (manage the background daemon),
  `update` (signed self-update).

> The `idapt-computer` binary handles the on-machine/daemon surfaces above. The
> full **resource grammar** (agents, chats, Drive CRUD, 200+ models, web search,
> triggers, …) lives in the npm [`@idapt/cli`](https://www.npmjs.com/package/@idapt/cli)
> and the [`@idapt/sdk`](https://www.npmjs.com/package/@idapt/sdk) — both speak
> the same `idapt <resource> <verb>` grammar over the public v1 API.

## Common commands

| Group | What it does |
|---|---|
| `auth` | `login` / `logout` / `status` |
| `up` / `down` | Bring this computer online / offline (daemon lifecycle) |
| `service` | Manage the background daemon (`up`, `down`, `restart`, `status`, `logs`) |
| `drive` | FUSE-mount your Drive + cloud sync |
| `expose` / `unexpose` | Open / close a public tunnel to a local port |
| `ssh` / `tunnel` | SSH into a computer through idapt; inspect tunnels |
| `local-inference` | Install / start / pull / run private on-device models |
| `update` / `uninstall` | Self-update (signature-verified) / remove from this machine |
| `completion` | Shell autocompletion scripts |

Run `idapt help <command>` for arguments and `idapt instructions <command>` for a
when/why playbook.

## Global flags

```
--api-key string    API key (or IDAPT_API_KEY env)
--api-url string    API base URL (default https://idapt.app)
-o, --output        Output format: table | json | jsonl | quiet
--confirm           Skip confirmation prompts for destructive ops
--verbose           Show request/response details
```

## Updates

`idapt update` checks for new releases, verifies each binary's SHA-256 + Ed25519
signature against an embedded trust root, and atomically swaps itself in place.
Releases are staged-rollout aware, so upgrades arrive gradually.

## Documentation

- [CLI overview](https://idapt.app/cli)
- [Help center](https://idapt.app/help/cli-overview)
- [Public API reference](https://idapt.app/api/v1/docs)

## License

Apache-2.0 — see [LICENSE](./LICENSE), [NOTICE](./NOTICE), and
[THIRD_PARTY_NOTICES.txt](./THIRD_PARTY_NOTICES.txt).
