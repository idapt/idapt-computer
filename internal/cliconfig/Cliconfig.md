# cliconfig

Persisted CLI configuration at `~/.idapt/config.json`. Small, human-editable,
versioned. Read on every command via `cmd/root.go`'s `PersistentPreRunE`.

## Schema (v1)

| Field            | Type   | Purpose                                                |
|------------------|--------|--------------------------------------------------------|
| `version`        | int    | Schema version stamp (currently `1`).                  |
| `apiUrl`         | string | Base URL of the idapt API. Defaults `https://idapt.ai`. |
| `defaultWorkspace` | string | Workspace slug/id used when no `--workspace` is passed.    |
| `outputFormat`   | string | One of `table`, `json`, `jsonl`, `quiet`.              |
| `noColor`        | bool   | Suppress ANSI colors (alternative: `NO_COLOR=` env).   |

## Override precedence

For each field, the effective value comes from (first wins):

1. Command-line flag (`--api-url`, `-w`/`--workspace`, `-o`/`--output`, `--no-color`).
2. Environment variable (`IDAPT_API_URL`, `IDAPT_WORKSPACE` — with `IDAPT_PROJECT`
   kept as a deprecated alias, `IDAPT_WORKSPACE` winning when both are set —
   `IDAPT_OUTPUT`, `NO_COLOR`).
3. The persisted config file (`defaultWorkspace`, set via `idapt workspace use`
   or `idapt config set`; clear with `idapt config unset defaultWorkspace`).
4. Compiled-in default. For the workspace specifically, this is the reserved
   `personal` slug (see `cmd.DefaultWorkspaceSlug`) — a missing workspace never
   errors, it falls back to the user's own personal workspace.

The CLI also exposes `idapt config path` (print the file location), `idapt config
edit` (`$VISUAL`/`$EDITOR`), and `idapt config unset <key>` (reset a key to its
default) alongside `get`/`set`/`list`.

## Versioning + migration

`CurrentSchemaVersion = 1`. On `Load`:

* If `version > CurrentSchemaVersion` → fall back to defaults (a future build
  wrote this; we don't know its semantics, so don't risk re-saving with
  truncated state). No error returned — the file is preserved on disk.
* If `version == 0` (pre-versioning) or omitted → migrate forward in memory by
  stamping `CurrentSchemaVersion`. The same fields all parse cleanly because
  the schema is purely additive so far.
* If `version` is unparseable JSON → fall back to defaults silently. The next
  `Save` will overwrite cleanly.

## Concurrent-write safety

`Save` is concurrency-safe across `idapt` processes via two layers:

1. **Advisory file lock** (`flock(LOCK_EX)` on a sibling `.lock` file). Two
   `idapt` processes saving the same config serialize cleanly.
2. **Atomic rename**: writes to a temp file in the same directory and renames.
   Readers that race with `Save` see either the old or new file, never an
   in-progress write.

On Windows the file-lock helper is a no-op; the atomic rename still gives
readers a consistent view, and concurrent writes are rare on a desktop
platform.

## Why no encryption

The file holds no secrets — credentials live in `~/.idapt/credentials.json`
(see `internal/credential/`). This separation lets the config be backed up,
synced, and edited freely without exposing the API key.
