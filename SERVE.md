# services/idapt — Daemon Architecture

The Idapt CLI binary doubles as a per-computer **daemon** when invoked as
`idapt serve`. The daemon is the app's only control-plane peer for managed
computers — all command execution flows through the SSE+POST channel
implemented in `internal/commands/`.

## Process layout

```
                               ┌────────────────────────────┐
                               │ idapt serve                 │
                               │   (cloud computer / BYO)   │
                               └────────────────────────────┘
                                          │
            ┌─────────────────────────────┼─────────────────────────────┐
            │                             │                             │
            ▼                             ▼                             ▼
  ┌──────────────────┐         ┌──────────────────┐         ┌──────────────────┐
  │ SSE client       │         │ Heartbeat        │         │ Tunnel client    │
  │ (commands)       │         │ goroutine        │         │                  │
  │                  │         │                  │         │ → WSS to the     │
  │ → executor pool  │         │ → POST /heartbeat│         │   tunnel-proxy   │
  │ → result poster  │         │   every 30s      │         │ → 127.0.0.1:port │
  └──────────────────┘         └──────────────────┘         └──────────────────┘

The daemon serves no public traffic of its own — public requests reach a
computer only through the central tunnel-proxy. It exposes one small plain-HTTP
management API (firewall + tunnels + health), HMAC-authenticated.
```

## Key packages

- `cmd/serve.go` — wires everything; resolves the config path via
  `config.ResolveConfigPath` so the per-user `~/.config/idapt/config.json`
  is preferred over the legacy `/etc/idapt/config.json` while keeping
  cloud-init-provisioned cloud computers running unchanged.
- `cmd/up.go` — `idapt up`, the canonical "make this computer work"
  verb. Idempotent end-to-end: install autostart unit (unless
  `--no-service`), authorize via the Tailscale-style device flow
  (default), write the daemon config to the per-user XDG path, then
  start the daemon. Multi-account guard refuses to overwrite an
  existing pairing without `--force`. Two non-default modes:
  `--token <pt_…>` triggers the legacy pair-token flow (install
  script / CI / mass-provision); `--code ABCD-2345` polls a
  pre-minted device code from the web UI without re-minting one.
  `idapt login` is registered as a verbatim alias.
- `cmd/lifecycle_aliases.go` — `idapt down` (top-level alias of
  `service down`) and `idapt logout` (clear the per-user daemon
  config + stop the daemon; orthogonal to `idapt auth logout`, which
  clears the API key in `credentials.json`).
- `cmd/pair.go` — legacy `idapt pair --token=...`. Still works (the
  install one-liner shells out to it), but prints a deprecation
  notice steering new users at `idapt up`.
- `internal/deviceflow/` — client for the Tailscale-style device
  flow: `Mint`, `PollOnce`, `Poll`. Surfaces `PollApproved /
  PollDenied / PollExpired / PollNotFound / PollCanceled` so the CLI
  can branch without string-matching.
- `internal/config/path.go` — `UserConfigPath` /
  `ResolveConfigPath` — single source of truth for "where does
  config.json live", with the legacy `/etc/idapt/config.json`
  fallback wired in.
- `cmd/selftest.go` — pre/post-update health probe. Returns non-zero if
  required system binaries (bash, runuser, prlimit) are missing.
- `cmd/service.go` (+ per-OS `cmd/service_{linux,darwin,windows}.go`) —
  granular daemon lifecycle scoped under `idapt service`:
  `up` (idempotent install+start), `down` (stop), `restart`, `status`,
  `logs` (`-f`, `--since`, `--lines`), `uninstall` (rare — removes
  the autostart unit entirely). Each verb maps to `systemctl --user`
  (Linux) / `launchctl` (macOS) / Task Scheduler (Windows — stubbed
  pending wiring). `idapt up` calls `service up` internally, so most
  users never need to type the sub-namespaced verbs.
- `internal/commands/` — SSE subscriber + executor for the daemon command
  channel. See `internal/commands/COMMANDS.md`.
- `internal/heartbeat/` — daemon→app heartbeat sender.
- `internal/auth/` — computer-token HMAC validation for inbound management calls.
- `internal/tunnelclient/` — the outbound tunnel data plane: holds the WSS to
  the tunnel-proxy, accepts multiplexed streams, forwards each to a local port.
- `internal/firewall/` — iptables management for the computer.
- `internal/revoke/` — invoked on three consecutive 401s; wipes config and
  exits cleanly so systemd doesn't restart.
- `internal/update/verify.go` — Ed25519 signature verification for binary
  self-update.
- `GET /api/health` — returns daemon health plus `commandsEnabled`,
  `commandsConnected`, and `commandsLastError`. `commandsEnabled` confirms a
  computerToken-backed command subscriber was configured at startup;
  `commandsConnected` confirms the SSE command stream is currently open.
  Test-mode containers also serve this path and `/__test/*` control endpoints
  on the HTTP port so sibling Kubernetes test-runner pods can configure the
  harness without TLS.

## Configuration

`/etc/idapt/config.json` (mode 0600) carries the **cloud-computer pairing**:

```json
{
  "computerId": "uuid",
  "appUrl": "https://idapt.app",
  "domain": "{slug}.idapt.app",
  "computerToken": "<hex hmac secret>",
  "tunnelProxyUrl": "wss://tunnel.idapt.app",
  "defaultBackendPort": 80,
  "defaultUser": "ubuntu"
}
```

Env-var overrides (highest precedence) listed in `internal/config/config.go`.

### Local mode (no config file)

When `/etc/idapt/config.json` is missing — the common case for `idapt
service up` on a personal Linux/macOS machine — the daemon boots in
**local mode**:

- `config.Load()` returns a defaults-only `Config` (the `IsLocalMode()`
  helper reports it).
- The cloud-only paths are skipped at startup: heartbeat, SSE command
  channel, tunnel client, computer-side `/api/firewall` handler.
- The management API binds **127.0.0.1:6480** instead of `:80` — a
  loopback bind + an unprivileged port — so a user-scope systemd unit
  can start it without root.
- Only the health endpoint (`GET /api/health`) and any configured FUSE
  mounts run. Everything else is a no-op until the operator pairs the
  computer (`idapt pair --token …`).

A present-but-empty config (`{}`) is still treated as cloud mode and
fails loud — local mode is reserved for the *no file at all* case so a
half-written cloud config cannot silently downgrade.

## Lifecycle

1. **Provision** — cloud-init writes config + binary, enables `idapt.service`.
2. **First run** — daemon starts heartbeat + SSE subscriber + tunnel client.
3. **Steady state** — receives commands, executes, posts results.
4. **Update** — `idapt update` runs every 6h; signed binary verified.
5. **Failure** — systemd `OnFailure=idapt-recover.service` restores
   last-known-good binary.
6. **Revoke** — three 401s → `revoke.Trigger()` → wipe + exit.

## Security properties

- **runAs validation** — `internal/commands/runuser.go` enforces POSIX
  username regex + refuses root unless explicit policy + refuses `_daemon`.
- **HMAC outbound** — heartbeat + result POSTs signed with computerToken.
- **HMAC inbound** — `internal/auth/hmac.go` validates app→daemon signatures
  on `/api/firewall` and `/api/tunnels`; signed timestamps must be within the
  freshness window to prevent replay.
- **File mutation boundary** — file write/delete/mkdir/move commands are
  constrained to the `runAs` user's home directory, following symlinks before
  policy checks. When the daemon runs as root, newly written paths are chowned
  back to `runAs`.
- **Resource caps** — systemd MemoryMax/CPUQuota/TasksMax + per-command
  `prlimit` wrap.
- **No persistent secrets in command output** — daemon-side secret
  materialization with tmpfs cleanup; file-mode secrets are owned by `runAs`
  with `0400` permissions while the command runs; app-side audit redaction.
