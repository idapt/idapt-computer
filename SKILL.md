---
name: idapt-cli
description: idapt CLI — manage projects, agents, files, chats, tasks, secrets, sub-agents, and 200+ AI models from the terminal. Use `idapt help <res> <verb>` for the per-verb contract and `idapt instructions <res>` for the resource playbook (or any verb's `--instructions` flag); both are lazy, fetch only what you need. In the in-app dispatcher, destructive verbs (e.g. `file delete`) gate on instructions and refuse the first call with the playbook body inline.
icon: terminal
version: "1.0"
license: MIT
---

# idapt CLI

Command-line for the idapt platform. Format: `idapt <resource> <verb> [--kebab-case-flags]`. Mirrors the in-app dispatcher 1:1: same verbs, args, and playbook bodies on both surfaces.

## Install + auth

```bash
curl -fsSL https://idapt.ai/cli/install | bash
idapt auth login                              # or: idapt config set api_key uk_...
```

## Discovery — lazy, asymmetric

Two parallel doc surfaces, same navigation shape, different tier models:

| Surface | Tier | Use when |
|---|---|---|
| `help` (CONTRACT) | per-verb (cobra-style) | "what args does THIS verb take?" |
| `instructions` (PLAYBOOK) | resource-scoped (one body per resource) | "should I use this resource and how to wield it well?" |

```bash
idapt help                                    # Top-level command index
idapt help <resource>                         # Resource's verbs (e.g. `idapt help file`)
idapt help <resource> <verb>                  # Per-verb CONTRACT — args, types, errors
idapt <resource> <verb> --help                # Same, flag-style (cobra builtin)

idapt instructions                            # Top-level playbook index
idapt instructions <resource>                 # Resource PLAYBOOK — canonical
idapt instructions <resource> <verb>          # Same body + footer note ("playbook is resource-scoped")
idapt <resource> <verb> --instructions        # Same, flag-style — short-circuits any command
```

### Decision flow

1. Don't know what's available? → `idapt help`
2. Know the verb, unsure about args? → `idapt help <res> <verb>` or `--help`
3. Know the call shape, unsure if it's the right move? → `idapt instructions <res>` or `--instructions`

### Destructive verbs gate on instructions (in-app dispatcher)

In the in-app dispatcher, verbs like `file delete`, `trigger fire`, `machine manage`, `trigger rotate-secret` refuse the first call with `INSTRUCTIONS_REQUIRED` if the playbook hasn't been fetched this session. The body is returned inline on the failed call — recovery is one extra round-trip, never two. The CLI uses an interactive `--confirm` prompt for the same destructive operations; reading `idapt instructions <res>` first is recommended.

## Resources

`project`, `agent`, `file`, `chat`, `task`, `secret`, `notification`, `subagent`, `web`, `media`, `exec`, `model`, `share`, `auth`, `config`, `api-key`. Run `idapt help <resource>` for that resource's verbs.

Some resources require additional features to be enabled on your account (e.g. machines, triggers, desktop). Run `idapt help` to see exactly what's available to you — it filters to your current entitlements.

## Conventions

- Flag names are kebab-case (`--new-chat`, `--system-prompt`). Boolean flags don't need a value.
- Output format via `-o {table|json|jsonl|quiet}`. `quiet` returns IDs only — `ID=$(idapt chat create -o quiet)` for scripting.
- Project scoping via `--project <slug>`; defaults to `personal`.
- API key: `--api-key uk_...` per-call, or `idapt config set api_key uk_...` globally.
- **Resource ids are 26-char Crockford strings** (e.g. `3f8hnkm2vp7qsgt0yca4bz5wrx`). The CLI accepts either a resourceId OR a friendly slug/name everywhere a resource is referenced — names get resolved against the `--project`. The response `id` is always the resourceId; internal UUIDs never appear on the wire.

## chat — branching, reprompt, cleanup ladder

Every chat is a TREE of messages. The CLI gives three ways to add to it:

```bash
idapt chat send <chat> "<text>"                       # append a new user turn at the active leaf
idapt chat send <chat> "<text>" --branch-from <msg>   # branch from an earlier message
idapt chat reprompt <chat> --message <user-msg-id>    # regenerate the assistant's reply (sibling)
```

`--effort-level fast|smart|expert` is the speed-dial: picks the
auto-router tier (Fast=Gemma-4-31b, Smart=MiniMax M2.7, Expert=GPT-5.5)
AND seeds the behavioural system reminder on every model. `--model`
overrides for this turn (`auto` engages the router). `--branch-from`
overrides the anchor. `--no-wait` returns immediately with a pending id.

Reprompt creates a SIBLING assistant — the original reply is preserved.
Use for model bake-offs:

```bash
idapt chat reprompt <chat> --message <user-id> --model anthropic/claude-opus-4.7 --effort-level expert
idapt chat reprompt <chat> --message <user-id> --model openai/gpt-5.5 --effort-level expert
idapt chat messages <chat> --last 10   # see all sibling responses
```

Cleanup ladder — prefer reversible:

```bash
idapt chat archive <chat>           # hide from sidebar; FULLY reversible via `unarchive`
idapt chat delete <chat>            # move to trash; reversible via `restore`
idapt chat permanent-delete <chat>  # IRREVERSIBLE; only on already-trashed
```

Run `idapt instructions chat` for the full playbook (covers cost
budget, auto+effort coupling, when to reprompt vs send-with-branch-from,
tier/FF gating behaviour).

## Links

- https://idapt.ai/cli — overview, install, docs
- https://github.com/idapt/idapt-cli — source
- https://idapt.ai/developers — developer hub
