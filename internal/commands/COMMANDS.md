# internal/commands — Daemon SSE+POST Command Channel

This package implements the daemon side of the wire protocol described in
[lib/computer-ops/computer-ops.md](../../../../lib/computer-ops/computer-ops.md).

## Files

| File | Purpose |
|---|---|
| `protocol.go` | Wire types (Envelope, Result, error codes). Mirrors `lib/computer-ops/protocol.ts`. |
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
| `local_inference.go` | Ollama status/install/start/stop/logs/model/prompt commands routed through daemon transport. |
| `computer_apps.go` | Docker-compatible Computer Apps runtime status/setup, image/Dockerfile/Compose launch, lifecycle, logs, exec, port inspection, and expose/unexpose. |
| `result.go` | HMAC POST of results back to the app. |

## Flow

1. `client.go` opens a long-lived `GET /api/computers/{id}/stream/commands`
   with HMAC headers + `Last-Event-ID`. The HTTP connect timeout applies only
   through response headers; once the SSE stream is accepted, it remains open
   until the app, network, or daemon closes it. Each event becomes an
   `Envelope`.
2. `Executor.Submit()` dedupes, queues into a 32-slot FIFO, semaphored at 8.
3. A worker picks up the task, builds the command via `runuser.go`,
   materializes secrets to env or tmpfs, runs with bounded output.
4. `result.go` POSTs the `Result` to `/api/computers/{id}/commands/{id}/result`
   with HMAC.

File-mode secrets live under `/run/idapt-secrets/{commandId}/{name}` only for
the lifetime of the command. The top-level directory is searchable, each
per-command directory is `0700`, and the files are `0400`, both owned by the
selected `runAs` user so `runuser` can read them while other users cannot.

## Resource caps

Per-command `prlimit` (Linux) caps:
- `--as=512M` (virtual memory)
- `--cpu=300` (5min CPU time)
- `--nofile=1024`, `--fsize=1G`

RLIMIT_NPROC is deliberately not set: the kernel counts it per real-UID
system-wide, and the exec path forks through `runuser` while still root, so the
cap would land against UID 0's global process count and fail with `EAGAIN` on
shared-kernel hosts (k3d test nodes, busy computers). Runaway process trees are
bounded instead by the per-command timeout + process-group kill.

`exec` and `file-*` use the defaults; `file-upload` / `file-download` get
`fsize=10G`; `port-discover` / `health` get tighter caps. The command TTL
ceiling is 30 minutes so long-running daemon-native operations such as local
inference runtime installs can complete on slower networks; shell CPU time
remains capped separately by `prlimit`.

## runAs safety

`ValidateRunAs()` is called as the first step of every handler:

1. Regex `^[a-z_][a-z0-9_-]{0,31}$`
2. `_daemon` is reserved (rejected from external)
3. `root` requires `RunuserConfig.AllowRoot`
4. `/etc/passwd` lookup confirms user exists (Linux only)

The app side already runs the equivalent check via `assertCanRunAs` in
`lib/computer-ops/runas-authz.ts`. This is defense-in-depth.

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

## Local inference

Local inference commands manage or use an Ollama runtime on the daemon host.
The user-facing CLI defaults to the locally paired daemon by reading the daemon
config's public `computerResourceId`; `--computer` targets a different daemon.
Managed install downloads the official Linux bundle into
`~/.idapt/local-inference/ollama/downloads`, extracts it into
`~/.idapt/local-inference/ollama/runtime`, keeps models under
`~/.idapt/local-inference/ollama/models`, and starts Ollama on a loopback
`OLLAMA_HOST` derived from the command's `baseUrl`. This avoids conflicts with
an existing system Ollama install. Operators can point at an existing binary
with `IDAPT_OLLAMA_BINARY` (an absolute path to an executable file) or an
alternate managed root with `IDAPT_LOCAL_INFERENCE_HOME`.

The managed installer is resumable. It resolves the bundle metadata before the
download, writes partial bytes to `ollama.tar.zst.part`, stores source URL /
ETag / Last-Modified / size metadata next to the partial file, and resumes with
HTTP Range when the metadata still matches. Download progress is posted as
`progress` chunks with phase, byte counts, percent, speed, ETA, and resume
state. Extraction happens through a staging directory before atomically
replacing the runtime directory, so a cancelled extract does not leave a
half-installed binary tree. The daemon decodes `.tar.zst` with an embedded Go
decoder; host computers do not need a system `zstd` binary.

Chat calls POST to Ollama's OpenAI-compatible `/v1/chat/completions` endpoint
and relay the raw SSE bytes as `provider-sse` chunks. Model pulls relay Ollama
JSON progress lines as `progress` chunks. The final command result contains
only operational status, not prompt or response text.

Unit tests in `local_inference_test.go` cover URL validation, model listing,
and chat SSE chunk streaming with an HTTP fake. The Kubernetes integration
suite exercises the status command through the live daemon transport without
requiring model downloads.

## Computer Apps

Computer Apps commands manage Docker-compatible apps on the daemon host. The
app/backend authorizes the parent computer and sends `computer-app-*` command
kinds; the daemon runs Docker locally and returns runtime inspect data.

V1 supports runtime status/setup, single-container image launch, Dockerfile
build/run, Compose `up -d --build`, start/stop/restart/reset/delete, logs,
exec/shell, port inspection, read-only external-container listing, and
expose/unexpose through the existing tunnel CLI. Launches use Idapt labels,
resource limits, and no-new-privileges where the Docker command supports it.
Labels are metadata only; they do not change how the container runs. The
`idapt-apps` bridge network is reserved for preview/tunnel/discovery paths and
is not required for every app.

Compose policy validation blocks privileged mode, host namespaces, device
mounts, added Linux capabilities, the Docker socket, absolute host bind mounts,
and common credential directory mounts. This is a practical host-safety guard
for developer app workloads; it is not a strict malware-analysis VM boundary.
Warning-only findings such as Compose-published local ports or writable
project-root binds require explicit acceptance. Compose source files are not
modified; labels are added with temporary override metadata outside the
repository.

## Adding a new command kind

1. Add the constant + payload type to `protocol.go`.
2. Add a `runFooBar(ctx, env, cfg)` handler to a new or existing file.
3. Wire it in `executor.go runOne()` switch.
4. Mirror the type on the TypeScript side in `lib/computer-ops/protocol.ts`
   and add a method to `lib/computer-ops/dispatch.ts`.
5. Tests live alongside (`*_test.go`).
