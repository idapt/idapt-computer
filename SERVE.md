# services/idapt — Daemon Architecture

The Idapt CLI binary doubles as a per-machine **daemon** when invoked as
`idapt serve`. The daemon is the app's only control-plane peer for managed
machines — all command execution flows through the SSE+POST channel
implemented in `internal/commands/`.

## Process layout

```
                               ┌────────────────────────────┐
                               │ idapt serve                 │
                               │   (managed machine / BYO)   │
                               └────────────────────────────┘
                                          │
            ┌─────────────────────────────┼─────────────────────────────┐
            │                             │                             │
            ▼                             ▼                             ▼
  ┌──────────────────┐         ┌──────────────────┐         ┌──────────────────┐
  │ SSE client       │         │ Heartbeat        │         │ TLS reverse proxy │
  │ (commands)       │         │ goroutine        │         │ + dynamic ports   │
  │                  │         │                  │         │ + JWT auth        │
  │ → executor pool  │         │ → POST /heartbeat│         │ (existing serve)  │
  │ → result poster  │         │   every 30s      │         │                   │
  └──────────────────┘         └──────────────────┘         └──────────────────┘
```

## Key packages

- `cmd/serve.go` — wires everything; loads `/etc/idapt/config.json`.
- `cmd/pair.go` — `idapt pair --token=...` exchanges a one-time token for the
  long-lived machineToken. Writes `/etc/idapt/config.json`.
- `cmd/selftest.go` — pre/post-update health probe. Returns non-zero if
  required system binaries (bash, runuser, prlimit) are missing.
- `cmd/service_install.go` (+ per-OS files) — installs systemd / launchd /
  Windows Service units.
- `internal/commands/` — SSE subscriber + executor for the daemon command
  channel. See `internal/commands/COMMANDS.md`.
- `internal/heartbeat/` — daemon→app heartbeat sender (existing).
- `internal/auth/` — HMAC + JWT + JWKS primitives (existing).
- `internal/proxy/`, `internal/firewall/`, `internal/listener/` — user-traffic
  reverse proxy + per-port TLS listener manager (existing).
- `internal/revoke/` — invoked on three consecutive 401s; wipes config and
  exits cleanly so systemd doesn't restart.
- `internal/update/verify.go` — Ed25519 signature verification for binary
  self-update.
- `GET /api/health` — returns daemon health plus `commandsEnabled`,
  `commandsConnected`, and `commandsLastError`. `commandsEnabled` confirms a
  machineToken-backed command subscriber was configured at startup;
  `commandsConnected` confirms the SSE command stream is currently open.
  Test-mode containers also serve this path and `/__test/*` control endpoints
  on the HTTP port so sibling Kubernetes test-runner pods can configure the
  harness without TLS.

## Configuration

`/etc/idapt/config.json` (mode 0600) carries:

```json
{
  "machineId": "uuid",
  "appUrl": "https://idapt.app",
  "domain": "{slug}.idapt.app",
  "jwksUrl": "https://idapt.app/api/managed-machines/jwks",
  "machineToken": "<hex hmac secret>",
  "defaultBackendPort": 80,
  "defaultUser": "ubuntu"
}
```

Env-var overrides (highest precedence) listed in `internal/config/config.go`.

## Lifecycle

1. **Provision** — cloud-init writes config + binary, enables `idapt.service`.
2. **First run** — daemon starts heartbeat + SSE subscriber + reverse proxy.
3. **Steady state** — receives commands, executes, posts results.
4. **Update** — `idapt update` runs every 6h; signed binary verified.
5. **Failure** — systemd `OnFailure=idapt-recover.service` restores
   last-known-good binary.
6. **Revoke** — three 401s → `revoke.Trigger()` → wipe + exit.

## Security properties

- **runAs validation** — `internal/commands/runuser.go` enforces POSIX
  username regex + refuses root unless explicit policy + refuses `_daemon`.
- **HMAC outbound** — heartbeat + result POSTs signed with machineToken.
- **HMAC inbound** — `internal/auth/hmac.go` validates app→daemon signatures
  on `/api/firewall` and `/api/proxy`; signed timestamps must be within the
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
