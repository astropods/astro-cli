# Docs-authoring skills for docs-public

## Summary

Writers (and Claude) had no encoded conventions for the public docs
(`docs-public/`, a Fern site): voice, frontmatter, component usage, and link
construction drifted, and the public CLI reference had to be hand-synced with
the `ast` CLI with nothing to check it. This adds a layered set of authoring
aids so docs work is consistent and checkable.

## Design

Three tiers, each with a single responsibility, so nothing is duplicated:

- **Mechanics — `fern-docs` skill.** Fern's official skill, covering generic Fern
  behavior: internal links, redirects/URL preservation, changelog wiring, product
  switchers, snippets, translations. It explicitly defers to the repo's own
  conventions, which makes the next tier the override. It is a third-party skill
  installed on demand (`npx skills add fern-api/skills --skill fern-docs -a
  claude-code`), **not vendored** — it has its own upstream, license, and
  full-agent-permission surface. `.claude/skills/README.md` documents the install
  and `.claude/skills/fern-docs/` is git-ignored.
- **House style — `docs-public/AGENTS.md`.** The source of truth for Astro's
  voice, page structure, frontmatter contract (`title`/`subtitle`/`slug`),
  component palette, and cross-referencing, adapted from Fern's own authoring
  guidance and grounded in the existing pages. A thin `docs-public/CLAUDE.md`
  points at it so Claude Code auto-loads it when working in that tree.
- **Astro-specific skills.**
  - `astro-docs-style` — checks copy against a distilled word/term list (brand
    terms, banned words, casing, US spelling); Query and Check modes.
  - `astro-docs-review` — per-page QA: `fern check` + frontmatter contract +
    style check + structural conventions.
  - `astro-audit-cli-docs` — three-way drift audit between the `ast` CLI's
    recursive `--help`, the public `cli-reference.mdx`, and the internal
    `cli-command-tree.md`, directly serving the manual CLI-sync mandate in
    `apps/astro-cli/CLAUDE.md`.

Most Postman tech-writer skills were deliberately **not** ported: page
move/rename/delete and redirects are covered generically by `fern-docs`
(and Astro is single-version, without Postman's dual `v11` tree), and the rest
are bound to Confluence/Jira/Looker infrastructure Astro doesn't have. Only the
three skills carrying reusable writing craft were adapted, stripped of
Postman-isms (`dnt`/Vale linter, `topictype` taxonomy, the Confluence sync mode,
hard-coded Jira field IDs).

To ship our skills as shared team assets, `.gitignore` tracks `.claude/skills/`
while still ignoring `.claude/settings.local.json`, `.claude/worktrees/`, and the
third-party `.claude/skills/fern-docs/`.

Two conventions in `AGENTS.md` were tuned to match existing practice: RFC/spec
reference pages (`astropods-package-spec`, `agent-card-spec`) may keep Title-Case
numbered headings, and changelog entries may carry `title`/`slug` alongside the
required `tags`. The CLI reference is scoped to the **prod `ast`** surface —
`ast knowledge`, top-level aliases, hidden commands, and dev-build-only commands
(e.g. `account token`) are intentionally excluded, and `astro-audit-cli-docs`
audits the prod/preview binary accordingly.

## Migration

None. New authoring aids only; no runtime or product surface changes. Writers
working under `docs-public/` will have the conventions loaded automatically and
can invoke the `astro-docs-*` skills by name.
