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
| `exec.go` | One-shot shell command (kind=exec); secret materialization (no-follow). |
| `files.go` | File ops (read/write/list/stat/mkdir/move/delete). |
| `grep_find.go` | file-grep + file-find (both run dropped-to-user). |
| `fsguard.go` | Privileged-group denylist + login-shell validation (portable). |
| `fsconfine_{linux,other}.go` | Symlink-safe, home-confined fd primitives (`openat2 RESOLVE_IN_ROOT\|RESOLVE_NO_SYMLINKS`, fd-relative mkdir/rename/unlink/chown). |
| `fsdrop_{linux,other}.go` | Re-exec the daemon dropped to the runAs user for read-family ops. |
| `fsop.go` | Read-family op spec/result + in-process executor (the `__fsop` child). |
| `tmux.go` | Tmux subcommands (run/capture/send-keys/list/kill). Sessions scoped per-runAs. |
| `users.go` | Unix user CRUD (create/delete/edit-groups). |
| `env.go` | Per-user env-var management via `~user/.bashrc.d/idapt-env`. |
| `ports.go` | Listening-port discovery: `/proc/net/tcp{,6}` on Linux, `netstat -an` fallback on macOS/Windows. |
| `health.go` | `health` command — returns version + concurrency stats + memory. |
| `local_inference.go` | Ollama status/install/**update**/start/stop/logs/model/prompt commands routed through daemon transport. |
| `ollama_dist.go` | Cross-platform engine artifact resolution + archive extraction (`.tar.zst` / `.tgz` / `.zip`) + binary discovery. |
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
`lib/computer-ops/runas-authz.ts`. This is defense-in-depth — the daemon
validates the **target** of every privileged op (path / username / group /
mount), not just the principal, because the daemon is root on cloud computers
and must not trust the control plane to have filtered.

### File ops: drop-for-read, confine-for-mutate

The daemon is root on cloud computers, so file ops follow two rules:

1. **Reads run AS the runAs user.** `file-read`, `file-list`, `file-stat`,
   `file-grep`, and `file-find` go through `dispatchFsRead`, which on a root
   daemon re-execs the binary as a hidden `__fsop` child with
   `SysProcAttr.Credential{Uid,Gid,Groups}` = the runAs user (`fsdrop_linux.go`
   + `fsop.go`). The kernel then enforces the user's own access, so the read
   family cannot leak a file the runAs user can't reach (e.g. `/etc/shadow`,
   another tenant's home). On a non-root daemon it runs in-process (the daemon
   user's uid bounds access; `runAs` is constrained to the daemon user). No
   `$HOME` allowlist — the privilege drop is the boundary.

2. **Mutations are symlink-safe + home-confined.** `file-write`, `file-delete`,
   `file-mkdir`, and `file-move` confine the target under the runAs user's home
   AND perform the act step via `openat2(RESOLVE_IN_ROOT|RESOLVE_NO_SYMLINKS)`
   fd-relative primitives (`fsconfine_linux.go`): mkdirat/renameat/unlinkat and
   `O_CREATE|O_EXCL` writes relative to a pinned parent dir fd, with
   `fchown`/`fchownat(AT_SYMLINK_NOFOLLOW)` for ownership. The kernel atomically
   refuses any symlink traversal and any escape above home at every component,
   closing the planted-`.idapt-tmp`-symlink escape and the parent-component swap
   TOCTOU. Atomic writes use an unpredictable tmp suffix. Symlinks are no longer
   traversed by mutations even within home (the home root is symlink-resolved up
   front by `resolveRunAsOwner`).

### Other target validations (daemon-side, defense-in-depth)

- **Env verbs** (`env-set`/`list`/`delete`): the env target `username` MUST
  equal the authorized `runAs` (`authorizeEnvTarget`). Login scripts of a user
  the actor isn't authorized to run as are never created/appended/chowned. The
  files are managed with the same symlink-safe confined primitives. The POSIX
  username regex is re-validated here too (parity with the user verbs).
- **User verbs**: `useradd`/`usermod` reject privileged groups
  (`docker`/`sudo`/`wheel`/`root`/`adm`/`lxd`/`kvm`/`disk`/`shadow`/…) via
  `validatePrivilegedGroups` (applied to the FULL resulting set, since
  `usermod -G` replaces it), reject `root` as a target, and validate the login
  shell to a clean absolute path. The run-as ACL is owned server-side; the
  daemon never auto-grants run-as.
- **Compose**: bind-mount sources are resolved relative to the project dir and
  confined inside it; relative `../` escapes and unexpanded `$VAR` sources are
  rejected (`checkMountSource`).

`exec` and `exec-stream` use a soft timeout from the payload when present; the
app-side envelope TTL includes scheduling and result-posting grace so a queued
command can still return structured `command-timeout` after it starts. The
`container` envelope field is reserved for future docker-exec support; current
command handlers reject non-empty container targets instead of running the
command on the host by accident.

Per-user environment variables are written to
`~user/.bashrc.d/idapt-env` with mode `0600` and ownership set to the target
user, allowing commands executed as that user to source the file while keeping
other users out. The target user is pinned to the authorized `runAs` (an editor
cannot target `root`'s home), and every open/create/chown of `.bashrc.d`,
`idapt-env`, `.bashrc`, and `.profile` goes through the symlink-safe confined
primitives so a planted symlink in the home is never written-through.

## Platform support

The daemon is built for Linux, macOS, and Windows from one source tree (pure Go,
`CGO_ENABLED=0`); platform differences are handled with build tags and runtime
`GOOS` branches, not forks. Commands are platform-aware: a feature that cannot
exist on a given OS returns a clear, explicit error instead of failing
obscurely. Notably:

- **Drive FUSE mount/sync** — Linux/macOS only (the FUSE package is build-tagged
  `linux || darwin`; on other platforms the `mount`/`unmount`/`sync` verbs are
  registered as stubs that explain they require FUSE).
- **Inbound firewall enforcement** — Linux/`iptables` only; macOS/Windows return
  a clear "supported on Linux only" error (`enforce_other.go`).
- **Seamless self-restart** — the SIGUSR1 + `exec()` in-place restart is Unix
  only; Windows defers the restart to the service manager (`serve_signals_*.go`,
  `update_{unix,windows}.go`).
- **Hardware detection, listening-port discovery, and managed Ollama** — each has
  per-OS implementations so they work on all three.

## Local inference

Local inference commands manage or use an Ollama runtime on the daemon host.
The user-facing CLI defaults to the locally paired daemon by reading the daemon
config's public `computerResourceId`; `--computer` targets a different daemon.
Managed install is cross-platform: it downloads the official Ollama bundle for
the host OS/arch — `ollama-linux-{amd64,arm64}.tar.zst` (Linux),
`ollama-darwin.tgz` (macOS, universal), or `ollama-windows-{amd64,arm64}.zip`
(Windows) — into `~/.idapt/local-inference/ollama/downloads`, extracts it into
`~/.idapt/local-inference/ollama/runtime` (the engine binary is located by
walking the extracted tree, so the differing per-platform layouts need no
hard-coding), keeps models under `~/.idapt/local-inference/ollama/models`, and
starts Ollama on a loopback `OLLAMA_HOST` derived from the command's `baseUrl`.
The dynamic loader for the bundled GPU runner libraries is wired per-OS
(`LD_LIBRARY_PATH` / `DYLD_LIBRARY_PATH` / `PATH`); unsupported OS/arch combos
return a clear, explicit error. This avoids conflicts with an existing system
Ollama install. Operators can point at an existing binary with
`IDAPT_OLLAMA_BINARY` (an absolute path to an executable file), an alternate
managed root with `IDAPT_LOCAL_INFERENCE_HOME`, or override the download URL
with `IDAPT_OLLAMA_DOWNLOAD_URL`.

The managed installer is resumable. It resolves the bundle metadata before the
download, writes partial bytes to `ollama.tar.zst.part`, stores source URL /
ETag / Last-Modified / size metadata next to the partial file, and resumes with
HTTP Range when the metadata still matches. Download progress is posted as
`progress` chunks with phase, byte counts, percent, speed, ETA, and resume
state. Extraction happens through a staging directory before atomically
replacing the runtime directory, so a cancelled extract does not leave a
half-installed binary tree. The daemon decodes `.tar.zst` and `.tgz` with
embedded Go decoders and `.zip` natively; host computers need no system
`zstd` / `tar` / `unzip` binary.

The `local-inference-runtime-update` verb (`idapt local-inference update`, or
the in-app **Update engine** button) re-downloads the latest bundle and swaps it
in, streaming the same `progress` chunks. A running engine is stopped first
(mandatory on Windows, where a live `.exe` cannot be replaced) and restarted on
the new version; models in the separate models dir are untouched. Update
detection is surfaced on `status` as `updateAvailable` / `latestVersion`,
computed by comparing the installed engine version against the newest published
Ollama release (best-effort, cached ~6h; overridable with
`IDAPT_OLLAMA_LATEST_VERSION`).

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
mounts, added Linux capabilities, the Docker socket, and common credential
directory mounts. Bind-mount sources are resolved the way Compose resolves them
(relative to the project directory) and confined INSIDE that directory:
relative `../` escapes and unexpanded `${VAR}`/`$VAR` sources are rejected, not
just absolute host paths — so the absolute-path denylist can't be bypassed by
expressing the same host path relatively. This is a practical host-safety guard
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
