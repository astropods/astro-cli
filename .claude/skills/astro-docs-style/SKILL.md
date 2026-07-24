---
name: astro-docs-style
description: >-
  Check Astro public docs against the house style and word list, or answer a
  question about what the style guide says. Trigger when a writer asks to check
  a page against the style guide, review word choice, audit language, verify
  terminology or capitalization, or asks whether something "follows our style" —
  even a bare "can you check this?" while an .mdx docs file is open. Also trigger
  for direct questions like "how do we capitalize AI Gateway?", "is 'utilize'
  allowed?", or "what's the preferred word for X?". Scope: docs-public/ (Fern
  MDX). Not for the Fern mechanics the fern-docs skill covers.
---

# Astro docs style

Two modes: **Query** (answer a style question) and **Check** (scan content and
report violations). Pick the mode from the request.

The term reference is `references/word-list.md` (brand terms, banned words,
capitalization, US spelling, formatting). Prose-level rules — voice, page
structure, components — live in `docs-public/AGENTS.md`; read it too when the
question is about voice or structure rather than a specific term.

## Mode: Query

The user asks what the style guide says ("do we capitalize AI Gateway?", "is
'leverage' banned?", "sentence case or title case for headings?").

1. Read `references/word-list.md`. If the question is about voice/structure,
   also read `docs-public/AGENTS.md`.
2. Answer directly and concisely, quoting the relevant rule or table row.
3. If nothing covers it, say so — do not invent a rule.

## Mode: Check

Scan one or more docs against the style rules and report violations. Inputs:

- **Single file or pasted text** — one report.
- **List of files** — one report block per file, then a summary.
- **Directory / glob** — use Glob to find `*.mdx` under the path; one report
  block per file, then a summary. Skip files under `.claude/`.

### How to run a check

1. Read `references/word-list.md` once, before reading any target files. Read
   `docs-public/AGENTS.md` too if you'll judge voice/structure.
2. Read all target files (in parallel if more than one).
3. For each file, scan for:
   - Terms in the **Avoid** columns (general terms, inclusive language).
   - Wrong **brand-term** forms (e.g. "Astro Pods", `Ast`, "AI gateway",
     "Knowledge Store").
   - Casing errors: non-sentence-case headings/titles/card titles; lowercase
     `ai`/`api`/`cli`/`yaml`/`url` in prose; unbackticked commands/flags/files.
   - British spellings (cancelled, behaviour, colour, …).
   - Formatting slips: UI labels not bold, code not backticked, click-paths not
     bolded segment-by-segment.
4. For voice/structure runs, also flag hedges/filler ("just", "simply", "in
   order to", "make sure you"), stacked callouts, and missing `## Next steps`
   on task/concept pages (per `AGENTS.md`).
5. Note near-misses (ambiguous cases) separately as a heads-up, not a hard
   violation.

Do not edit the file unless the user asks — Check reports; it doesn't fix.

### Output — single file

```
## Style check — <filename>

Violations found: N

### 1. <flagged term or issue>
> "…the sentence containing it…"

❌ Avoid: <what was used>
✅ Prefer: <recommended form>
```

If clean, say so briefly. For long files (>1000 words), note which sections were
checked.

### Output — multiple files

One block per file (single-file format above), then:

```
## Summary

| File | Violations |
|---|---|
| pages/foo.mdx | 3 |
| pages/bar.mdx | 0 ✅ |

X files checked. Y clean. Z total violations.
```

List clean files last with a ✅.
