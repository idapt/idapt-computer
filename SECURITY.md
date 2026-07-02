# Security Policy

The idapt computer daemon runs on your own machines with real privileges (it can
execute commands, mount your Drive, and open tunnels), so we take its security
seriously.

## Reporting a vulnerability

Please report vulnerabilities **privately**:

- Email **security@idapt.app**, or
- Use the contact at <https://idapt.app/security>.

Do not open public GitHub issues for security problems. Where possible, include
the affected version, a description of the issue, and steps to reproduce. We aim
to acknowledge reports within a few business days.

## Supported versions

Security fixes are released against the latest published version. The daemon
self-updates and verifies each release's SHA-256 + Ed25519 signature before
applying it. The update manifest is itself Ed25519-signed with a required expiry
and a monotonic anti-rollback counter.

## Daemon security controls

Because the daemon acts on your machine on behalf of a remote control plane, it
enforces its own boundaries rather than trusting the cloud for host safety:

- **Command surfaces are default-deny.** `commandPolicy` in `config.json` gates
  every risky family (shell, files, admin, computer-use, local-inference,
  terminal, tunnels). A forged/compromised cloud response cannot enable a
  surface — the enablement decision is local and authoritative.
- **runAs is bounded locally.** Root (`runAs` → UID 0) requires an explicit
  opt-in; the persisted `commandPolicy.allowRunAsRoot` is authoritative when set
  (so the operator toggle actually disarms the daemon, overriding the legacy
  `IDAPT_ALLOW_RUNAS_ROOT` env). `commandPolicy.restrictRunAs` +
  `commandPolicy.allowedRunAs` restrict cross-user targets to a daemon-side
  allowlist so the control plane cannot run commands as arbitrary local accounts.
- **Child-command environments are sanitized.** The daemon's own secrets
  (`IDAPT_*` token/key) and loader/shell-injection keys (`LD_*`, `BASH_ENV`,
  `BASH_FUNC_*`, …) are stripped from every executed command, so an AI-run
  command cannot exfiltrate the machine-control credential or inject code.
- **Filesystem writes are confined.** Drive/FUSE writes go through
  `openat2(RESOLVE_IN_ROOT|RESOLVE_NO_SYMLINKS)`; server-supplied file names
  never build a raw host path, and the disk cache is private (0700 dirs / 0600
  blobs) so co-tenant users cannot read another workspace's cached content.
- **Transport is confidential.** All cloud traffic uses a TLS 1.2+ floor and a
  non-loopback `http://` API/app URL is refused (self-hosting override:
  `IDAPT_ALLOW_INSECURE_APP_URL=1`).
- **Revocation is enforced.** Deleting/revoking a computer returns an
  authoritative signal (HTTP 410) that makes the daemon wipe its local
  credential; a fleet-wide transient rejection cannot self-wipe the fleet.

Report a vulnerability in any of these to **security@idapt.app**.
