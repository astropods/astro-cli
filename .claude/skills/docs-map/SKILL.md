---
name: docs-map
description: >-
  Look up which files under docs/ (spec, architecture, guides, plans — not
  docs-public/) are canonical for a feature area, before explaining how a
  system works, starting work in an area with existing docs, or checking
  whether a doc needs updating after a change. Use when asked "how does X
  work", "where is X documented", when a task's own description names a
  system this repo's area map covers (billing, auth, org/FGA, registry auth,
  knowledge stores, astro-cli — check docs/README.md for the current list,
  it grows over time), or as the last step of a task that changed behavior
  in one of those areas.
  For docs-public/ (the published product docs), use astro-docs-review or
  astro-docs-style instead — this skill never touches that tree.
---

# docs-map

Answers one question: which doc(s) under `docs/` should I read (or fix),
for a given area — so a task pulls in a tight, targeted set of files
instead of grepping 90+ across `01-spec`, `03-architecture`, `04-guides`,
`05-implementation`, `06-plan`, `07-feedback`, `08-incidents`, `09-reference`.

## Retrieving context for an area

1. Read [`docs/README.md`](../../../docs/README.md)'s "Area → canonical doc
   map" table.
2. **Area listed:** read exactly the doc(s) named there, in the order given
   (they're usually layered short→detailed). Treat them as ground truth for
   the task. Don't also open other docs/ files for the same area "just in
   case" — the map exists so you don't have to.
3. **Area not listed:** say so plainly rather than guessing a doc is
   authoritative. Do a targeted search restricted to `01-spec/` and
   `03-architecture/` (skip `06-plan/`, which is proposals, and
   `changelog/`, which is history, unless nothing else turns up anything).
   If something relevant turns up, that's a candidate to add to the map —
   mention it, don't add it unprompted mid-task.
4. `04-guides/` (how-to) and `09-reference/` (third-party contracts) aren't
   part of the area map since they're not "how a system works" docs; grep
   them directly by filename/topic when a task needs a specific guide.

## Checking docs after a change

When a task ends having changed behavior in a mapped area:

1. Re-read the canonical doc(s) for that area (step 2 above).
2. If anything in them now reads as wrong given the change just made, fix it
   in the same set of edits — per `docs/README.md`'s rule, a living doc that
   disagrees with the code is a defect in the same PR, not a follow-up.
3. If fixing it needs domain knowledge outside the current task, add one
   entry to [`docs/07-feedback/doc-drift-log.md`](../../../docs/07-feedback/doc-drift-log.md)
   instead of leaving it unrecorded. Keep the entry to what/where/why-deferred;
   don't turn the log into a second backlog.
4. If the change doesn't touch anything the canonical docs claim, no action —
   don't add a doc-touch to a PR that doesn't need one.

## Output

State which doc(s) were read (or which area came back unmapped), in one
line, before using their content — so it's clear the answer is scoped to
those files rather than general knowledge of the codebase.
