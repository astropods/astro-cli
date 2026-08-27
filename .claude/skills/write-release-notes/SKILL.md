---
name: write-release-notes
description: >-
  Cut a new release note in docs/releases/ and tag it. Use when asked to
  "cut a release", "write release notes", "create a new release", "what's
  changed since the last release", or when starting the release process
  described in agents.md's Releases section. Not for docs/changelog/ (see
  write-changelog) and not for docs-public/.
---

# write-release-notes

Encodes the process in `agents.md`'s Releases section as executable steps.
Each release is one file, `docs/releases/YYYY-MM-DD.N.md`, `N` starting at
`1` per day, tagged `release/YYYY-MM-DD.N` to match.

## 1. Find the commit range

```bash
ls docs/releases/ | sort | tail -1
```

Open that file's Appendix and read its **Commit range** line — the new
range starts at that range's end commit and runs to `HEAD`:

```bash
git log <previous-end-commit>..HEAD --oneline
```

If `docs/releases/` is empty (the first release ever), there's no prior
end commit to start from — ask the user for the starting point (a commit,
tag, or "since the beginning of history") rather than guessing one.

## 2. Read every changelog in range

```bash
git log <previous-end-commit>..HEAD --name-only --diff-filter=A -- docs/changelog/
```

Read each one. They carry the design context this release note is built
from — don't re-derive it from raw commit messages when a changelog already
explains the why.

## 3. Check docs/README.md's area map for staleness

This is the one point in the normal workflow where a whole commit range's
worth of changes gets looked at together, so it's the cheapest place to
catch drift a single-PR pass would miss. For each area the changelogs in
range touch:

1. Check whether it's listed in [`docs/README.md`](../../../docs/README.md)'s
   area map (same lookup the `docs-map` skill does).
2. If listed, and the canonical doc no longer matches what shipped, fix it
   in the same pass — same rule as any other living doc.
3. If an area isn't mapped yet but the changelogs suggest it now has a
   clear canonical doc, that's a candidate to add — mention it, don't add
   it unprompted mid-release.

## 4. Determine the filename

```bash
date +%F   # YYYY-MM-DD
ls docs/releases/ | grep "^$(date +%F)\."   # existing releases today, if any
```

`N` is one past the highest existing counter for today's date, or `1` if
none exist yet.

## 5. Write the release note

Two sections, in this order:

**Public section (top)** — user-facing, no internal jargon:
- Feature headers describe what users can now do, not how.
- Bullet points describe observable behavior, not implementation.
- No component names, API paths, DB details, env vars, or variable names.
- A "Fixes" grouping covers user-visible regressions only — an internal-only
  fix belongs in the appendix, not here.
- A migration table only if users need to take action; otherwise state
  plainly that nothing is required.

**Appendix (bottom, separated by `---`)** — internal, for the team:
- **Commit range** as the first line.
- Grouped by area (`### Billing`, `### Insights`, etc. — match the public
  section's grouping), one bullet per PR: PR number and branch name, then
  the technical specifics (component names, API params, DB/queue changes)
  and the root cause for any fix, not just what changed.

Follow `agents.md`'s house writing style throughout (active voice, no em
dashes, no narrated session history). For calibration on register and
density, read the most recent file in `docs/releases/` in full before
writing — it's real, shipped output at the length and level of detail this
should match, not a template to fill in mechanically.

## 6. Commit and tag

```bash
git add docs/releases/YYYY-MM-DD.N.md
git commit -m "..."
git tag release/YYYY-MM-DD.N
```

The tag must match the filename exactly, including the counter.

## Output

State: the file path, the tag name, the commit range covered, and whether
step 3 found (and fixed) any stale canonical docs.
