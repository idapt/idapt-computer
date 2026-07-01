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
applying it.
