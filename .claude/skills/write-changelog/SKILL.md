---
name: write-changelog
description: >-
  Write or update the docs/changelog/ entry required on every PR in this
  repo. Use when finishing a branch of work, before opening a PR, or when
  asked to "add a changelog", "write the changelog for this", "what's the
  changelog filename for this branch", or a CI comment says a PR is missing
  one. Not for docs/releases/ (see write-release-notes) and not for
  docs-public/ (see astro-docs-style/astro-docs-review).
---

# write-changelog

Every PR needs one file at `docs/changelog/{branch-name}-YYYY-MM-DD.md`. A
GitHub Action checks presence and filename only — it does not check content
shape, so getting the content right is on the author (or this skill).

## 1. Get the exact filename right

```bash
git branch --show-current
```

The filename must satisfy the CI's own pattern exactly:
`^{branch}-\d{4}-\d{2}-\d{2}\.md$`, checked relative to `docs/changelog/`.

- A branch with slashes (`fix/my-change`) maps to a subdirectory:
  `docs/changelog/fix/my-change-2026-03-10.md`.
- The date is when the file is written, not when the branch was created.
- If a changelog file for this branch already exists (earlier commit on the
  same branch), **edit it in place** rather than adding a second one — one
  file per branch, not one per commit.

## 2. Find the design content

```bash
BASE=$(git merge-base HEAD main 2>/dev/null || git merge-base HEAD origin/main)
git diff "$BASE"..HEAD --stat
git log "$BASE"..HEAD --oneline
```

(a shallow or single-branch checkout may have no local `main`; fall back to
`origin/main`)

Read the actual diff for the commits in range, not just the commit messages.
The changelog explains the system after the change and why it exists — it
has to come from what the code actually does, not from a paraphrase of
commit subjects.

## 3. Write it: Summary / Design / Migration

Per `agents.md`'s Changelogs rule:

- **Summary** — the problem being solved and why the change exists.
- **Design** — how the pieces fit together, key decisions, short code/config
  examples where helpful.
- **Migration** — what users need to do, or that nothing is required.

**Do not list individual file changes.** Explain the system, not the patch.
A changelog that reads `handlers/foo.go: added X` in a bullet list is the
anti-pattern this rule exists to prevent — say what X does and why it needed
adding, not that it was added.

Follow `agents.md`'s house writing style (active voice, one idea per
sentence, no em dashes, never narrate the session — no "we discovered",
no "as discussed", no dated status). Real example, for calibration on
length and register:

```markdown
# Dataset item write

## Summary

Adding a trace to the dataset went through the judge, which recorded one
good/bad verdict. Evaluators replace that verdict with one typed value per
evaluator, and nothing could store them.

This adds the endpoint that saves a trace to the dataset against the agent's
active evaluation set, with a verified value for every evaluator in it.

## Design

\`\`\`http
POST /api/v1/deployments/:id/dataset/items
\`\`\`

The request must carry a value for every evaluator in the active set, and
each is checked against that evaluator's declared output contract. A
missing evaluator, an unknown key, or a value the contract rejects fails
the request with a 400.

## Migration

Apply `sql/astro-server/schema.sql` before deploying astro-server. Adds
two tables. Additive, no backfill.
```

A changelog with no user-facing migration step still needs the section —
write "None." or "No action required," don't omit it.

## 4. Check whether this change touches a mapped doc area

If the diff touches a code path listed in
[`docs/README.md`](../../../docs/README.md)'s area map, that's a separate
concern from the changelog (see the `docs-map` skill) — don't skip it just
because you're mid-changelog, but don't fold doc fixes into the changelog
file either. They're two different files with two different jobs.

## Output

State the file path you wrote (or edited), and one line confirming the
commit range it covers.
