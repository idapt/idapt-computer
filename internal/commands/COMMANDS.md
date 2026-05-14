# internal/commands — Daemon SSE+POST Command Channel

This package implements the daemon side of the wire protocol described in
[lib/machine-ops/machine-ops.md](../../../../lib/machine-ops/machine-ops.md).

## Files

| File | Purpose |
|---|---|
| `protocol.go` | Wire types (Envelope, Result, error codes). Mirrors `lib/machine-ops/protocol.ts`. |
| `client.go` | SSE subscriber + reconnect loop with heartbeat watchdog and Last-Event-ID resume. |
| `executor.go` | Worker pool dispatch: switch on `kind`, semaphore + queue, dedup integration. |
| `runuser.go` | POSIX username validation + runuser/prlimit command builders. |
| `dedupe.go` | LRU cache for successful results (idempotency replay). |
| `exec.go` | One-shot shell command (kind=exec). |
| `files.go` | File ops (read/write/list/stat/mkdir/move/delete). |
| `grep_find.go` | file-grep + file-find. |
| `tmux.go` | Tmux subcommands (run/capture/send-keys/list/kill). Sessions scoped per-runAs. |
| `users.go` | Unix user CRUD (create/delete/edit-groups). |
| `env.go` | Per-user env-var management via `~user/.bashrc.d/idapt-env`. |
| `ports.go` | `/proc/net/tcp` parser (no ss/netstat dependency). |
| `health.go` | `health` command — returns version + concurrency stats + memory. |
| `result.go` | HMAC POST of results back to the app. |

## Flow

1. `client.go` opens a long-lived `GET /api/machines/{id}/stream/commands`
   with HMAC headers + `Last-Event-ID`. The HTTP connect timeout applies only
   through response headers; once the SSE stream is accepted, it remains open
   until the app, network, or daemon closes it. Each event becomes an
   `Envelope`.
2. `Executor.Submit()` dedupes, queues into a 32-slot FIFO, semaphored at 8.
3. A worker picks up the task, builds the command via `runuser.go`,
   materializes secrets to env or tmpfs, runs with bounded output.
4. `result.go` POSTs the `Result` to `/api/machines/{id}/commands/{id}/result`
   with HMAC.

File-mode secrets live under `/run/idapt-secrets/{commandId}/{name}` only for
the lifetime of the command. The top-level directory is searchable, each
per-command directory is `0700`, and the files are `0400`, both owned by the
selected `runAs` user so `runuser` can read them while other users cannot.

## Resource caps

Per-command `prlimit` (Linux) caps:
- `--as=512M` (virtual memory)
- `--cpu=300` (5min CPU time)
- `--nofile=1024`, `--nproc=512`, `--fsize=1G`

`exec` and `file-*` use the defaults; `file-upload` / `file-download` get
`fsize=10G`; `port-discover` / `health` get tighter caps.

## runAs safety

`ValidateRunAs()` is called as the first step of every handler:

1. Regex `^[a-z_][a-z0-9_-]{0,31}$`
2. `_daemon` is reserved (rejected from external)
3. `root` requires `RunuserConfig.AllowRoot`
4. `/etc/passwd` lookup confirms user exists (Linux only)

The app side already runs the equivalent check via `assertCanRunAs` in
`lib/machine-ops/runas-authz.ts`. This is defense-in-depth.

`file-write`, `file-delete`, `file-mkdir`, and `file-move` add a second
boundary: the target path must resolve under the selected `runAs` user's home
directory. Symlinks are resolved before the policy check, and root-owned daemon
writes are chowned back to `runAs` after creation.

`exec` and `exec-stream` use a soft timeout from the payload when present; the
app-side envelope TTL includes scheduling and result-posting grace so a queued
command can still return structured `command-timeout` after it starts. The
`container` envelope field is reserved for future docker-exec support; current
command handlers reject non-empty container targets instead of running the
command on the host by accident.

Per-user environment variables are written to
`~user/.bashrc.d/idapt-env` with mode `0600` and ownership set to the target
user, allowing commands executed as that user to source the file while keeping
other users out.

## Adding a new command kind

1. Add the constant + payload type to `protocol.go`.
2. Add a `runFooBar(ctx, env, cfg)` handler to a new or existing file.
3. Wire it in `executor.go runOne()` switch.
4. Mirror the type on the TypeScript side in `lib/machine-ops/protocol.ts`
   and add a method to `lib/machine-ops/dispatch.ts`.
5. Tests live alongside (`*_test.go`).
