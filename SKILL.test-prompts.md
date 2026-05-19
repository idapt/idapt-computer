# `idapt-cli` SKILL auto-load validation prompts

Sidecar to `SKILL.md`. Used to validate that the skill's `description`
frontmatter fires the auto-load matcher reliably when an agent is about
to use the idapt CLI. Not consumed at runtime — purely a manual /
offline eval target.

## How to use

When the hub auto-load infrastructure exists, run each prompt below
through the skill matcher and confirm:

- Prompts in **Should load** → the skill loads.
- Prompts in **Should NOT load** → the skill does NOT load (avoids
  spamming context for unrelated CLI work).

Acceptable miss rate target: < 5% on **Should load**, < 5% false-fire
on **Should NOT load**.

If the actual rate is worse than the target, edit the `description`
field in `SKILL.md` (more specific keywords, clearer trigger) and
re-run.

## Should load (idapt CLI is the right tool)

These prompts describe tasks the idapt CLI directly handles. The skill
should fire so the agent has the discovery protocol in scope before
running any commands.

1. "List my idapt projects."
2. "Run a Python script in my idapt sandbox."
3. "Send a message to one of my idapt agents from the terminal."
4. "Upload `report.pdf` to my idapt personal project."
5. "Search across all the files in my idapt workspace for `auth_token`."
6. "Create a new idapt task with title 'Refactor billing'."
7. "Fire the `nightly-report` idapt trigger now."
8. "Delete the `obsolete.md` file from my idapt project."
9. "Spawn a subagent to summarize this paper, using idapt."
10. "Generate an image with idapt media tools."
11. "Set up a webhook trigger in idapt."
12. "Mount my idapt secrets folder."

## Should NOT load (idapt CLI is irrelevant)

These prompts are about unrelated CLIs, generic shell, or other
platforms. The skill should NOT fire — loading it would waste context.

1. "Run `kubectl get pods`."
2. "Check the git log for the last 10 commits."
3. "What does `tar -xzvf` do?"
4. "Install `npm` packages."
5. "Use the GitHub CLI to create a pull request."
6. "Configure my AWS credentials."
7. "Run `docker compose up`."
8. "Format this Python code with `black`."
9. "How do I list S3 buckets?"
10. "Run `pytest` on the test suite."

## Borderline (judgment calls)

These overlap conceptually with idapt features but aren't unambiguous
CLI invocations. The skill firing or not is acceptable; document the
observed behavior so we know whether the matcher is over- or
under-tuned.

1. "I need to manage AI agents." (could be idapt, could be another platform)
2. "Run code in a cloud sandbox." (idapt has `exec`, but so do other tools)
3. "Send a notification to my team." (idapt has `notification`, but so do Slack / generic webhook tools)
4. "Search the web." (idapt has `web search`, but so does every assistant)
5. "Generate an image." (idapt has `media generate`, but so do many tools)

## Maintenance

Update this file whenever the SKILL `description` is edited. The prompt
set should evolve alongside the trigger wording — if a new keyword is
added (e.g. "model catalog", "fork project"), add a corresponding
should-load prompt to lock in the coverage.
