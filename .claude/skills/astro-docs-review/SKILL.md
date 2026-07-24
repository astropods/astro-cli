---
name: astro-docs-review
description: >-
  Run a maintenance review on an Astro docs page (.mdx under docs-public/):
  validation (fern check), frontmatter contract, style-guide compliance, and
  structural conventions. Use when a writer asks to "review", "audit", or
  "check" a docs page for issues, asks if a page is ready to ship, or asks what
  needs fixing on a page. For pure word-choice questions use astro-docs-style;
  for Fern mechanics use fern-docs.
---

# Astro docs review

Run a standard set of checks on one docs page and produce a consolidated report.
Adapted for Astro's Fern toolchain — there is no `dnt`/Vale linter and no
`topictype` taxonomy here; validation is `fern check`, and the frontmatter
contract is `title`/`subtitle`/`slug`.

If the user didn't name a file, ask which page to review before running checks.

## Checks

### 1. fern check

`fern check` validates the whole docs project (config, broken links, changelog
slugs, frontmatter Fern requires), not a single file. Run it once from the Fern
root and attribute any errors that reference the target page:

```bash
cd docs-public/fern && fern check
```

Record pass/fail. On failure, capture the errors touching the target file.

### 2. Frontmatter contract

Read the YAML frontmatter (between the opening/closing `---`). Per
`docs-public/AGENTS.md`:

- `title` — present, non-empty, **sentence case**.
- `subtitle` — present on a content page (one line).
- `slug` — present, non-empty.
- **Home/landing exception:** a page with `layout: overview` (e.g.
  `welcome.mdx`) legitimately omits `subtitle` and adds `hide-*` fields.
- **Changelog exception:** entries under `fern/docs/changelog/` require `tags`;
  `title` and `slug` are permitted (our entries use them). Check for a `tags`
  array; don't flag `title`/`slug` on changelog entries.

Flag missing/blank required fields, title-case titles, and any unsupported field
(`topictype`, `approved`, `max-toc-depth`, etc. are not used here). **RFC/spec
reference pages** (`astropods-package-spec`, `agent-card-spec`) may carry an
`RFC-N:` title prefix — not a violation.

### 3. Style guide

Run the `astro-docs-style` skill (Check mode) on the file and collect its
output verbatim.

### 4. Structure

Per `docs-public/AGENTS.md`:

- Task/concept pages **end with `## Next steps`** (bullet slug links or a
  `<CardGroup>`/`<Steps>`). Reference and changelog pages are exempt.
- **No two callouts stacked** back-to-back (`<Note>`/`<Warning>`/`<Tip>`/
  `<Callout>` with no prose between).
- Headings are **sentence case**; `##` for sections, `###` for subsections.
  Exception: RFC/spec reference pages (`astropods-package-spec`,
  `agent-card-spec`) may use Title-Case numbered headings.
- Internal links are **slug paths** (`/get-started`), not file paths
  (`./get-started.mdx`) or guessed folder paths.
- Components come from the established palette (Steps, CardGroup/Card, Tabs,
  CodeBlocks, Accordion, EndpointRequestSnippet). Flag ad-hoc raw HTML outside
  the home-page hero.

## Output

```
## Maintenance review: `<filename>`

### 1. fern check
✅ Passed
_or_ ❌ Failed — <errors touching this file>

### 2. Frontmatter
✅ title / subtitle / slug present, title is sentence case
_or_ ❌ <what's missing or wrong>

### 3. Style guide
<astro-docs-style output, verbatim>

### 4. Structure
✅ Next steps present · no stacked callouts · links are slug paths
_or_ ⚠️/❌ <specific issues>

---
Summary: <N> issue(s). <one sentence on what needs attention, or
"No issues found — page is ready.">
```
