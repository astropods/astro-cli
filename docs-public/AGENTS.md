# Astro docs authoring guide

House conventions for writing and editing Astro's public product docs (this
`docs-public/` tree, a [Fern](https://buildwithfern.com) site). This file is the
source of truth for **voice, structure, frontmatter, and components**. It is
adapted from Fern's own docs guidelines and grounded in the existing pages under
`fern/docs/pages/`.

## How the pieces fit

- **`fern-docs` skill** — generic Fern mechanics: internal-link construction,
  redirects/URL preservation, changelog wiring, product switchers, snippets,
  translations. Use it for the "how does Fern do X" questions. It defers to this
  file where they overlap. It is a third-party skill installed via npx, not
  vendored — see `.claude/skills/README.md` to install it.
- **This `AGENTS.md`** — Astro's house style. When it and the skill disagree on
  voice or structure, this file wins.
- **`astro-docs-style` skill** — checks prose against a word/term list.
- **`astro-docs-review` skill** — per-page QA (`fern check` + frontmatter +
  style + structure).
- **`astro-audit-cli-docs` skill** — keeps `cli-reference.mdx` in sync with the
  `ast` CLI.

The CLI reference documents the **prod `ast`** surface only: visible, non-hidden,
non-alias commands present in the prod build. Excluded from the public reference:
`ast knowledge`, the top-level aliases (`build`/`create`/`deploy`/`push`),
Cobra-hidden commands, and dev-build-only commands (e.g. `account token`, gated by
`buildinfo.BuildType == buildinfo.BuildTypeDev`).

## Core principles

- **Write what the reader needs to succeed — no more.** Every sentence earns its
  place.
- **Prefer editing over creating.** Search the tree for a page that already
  covers the topic and update it instead of adding a duplicate.
- **Make minimal, precise edits.** Don't rewrite a page when a paragraph fix
  will do.
- **Never fabricate.** If you don't know a flag, config key, or behavior, verify
  it against the code (`apps/`, `packages/astro-spec`) or say so. Don't invent
  frontmatter fields or spec keys.
- **Cross-reference.** When you mention a concept documented elsewhere, link to
  it.
- **Push back when something seems wrong.** Explain why rather than complying
  silently.

## Voice and style

- **Dry and direct.** State requirements plainly. Second person, imperative:
  "Install the CLI", "Push the blueprint".
- **Cut hedges and filler.** Remove "just", "simply", "make sure you", "you'll
  want to", "in order to", "so that". Prefer "X requires Y" over "make sure you
  have Y so you can do X".
- **Active voice.** "The CLI builds the image", not "the image is built".
- **Sentence case everywhere** — page titles, headings, card titles. (Matches
  every existing page: "Your first project", "What you can do".) Product names
  keep their own casing: "AI Gateway", "Astropods Spec". **Exception:** RFC/spec
  reference pages (`astropods-package-spec`, `agent-card-spec`) may keep
  Title-Case numbered section headings (`## 2. Top-Level Structure`) per RFC
  convention.
- **At most one em dash per short paragraph.** Use colons for lists, parentheses
  for asides.
- **Name things concretely.** "Set `visibility` in `astropods.yml`", not "as
  shown above".

Terminology (brand terms, banned words, capitalization) lives in the
`astro-docs-style` skill's `references/word-list.md`. Run that skill to check
copy.

## Callouts are the exception

Prose is the default. Reach for a callout only for a genuine aside — a secondary
consideration that would break the main flow if inlined.

- Available: `<Note>`, `<Warning>`, `<Tip>`, and `<Callout intent="note|info|tip|warning">`.
- **Never stack two callouts back-to-back.** Separate them with body prose.
- If the information belongs in the main flow, write it as a sentence, not a
  callout.

## Frontmatter contract

Content pages carry three fields, in this order:

```yaml
---
title: Your first project
subtitle: Create and run your first agent locally
slug: get-started
---
```

- `title` — sentence case, required.
- `subtitle` — one line, required on content pages.
- `slug` — the page's URL path segment, required. Astro links are flat
  (`/get-started`, `/install-cli`) — there is no product or base-path prefix.
- The **home page only** (`welcome.mdx`) adds `layout: overview` plus
  `hide-toc`, `hide-feedback`, `hide-nav-links` for its custom hero.
- **Changelog entries** take only `tags` (see below). No `title`/`slug`.

Do not add frontmatter fields that aren't in use here (no `topictype`,
`approved`, `max-toc-depth`, etc.).

## Component palette

Use the vocabulary already established in the tree. Pick the lightest component
that does the job.

| Component | Use for |
|-----------|---------|
| `<Steps>` / `<Step title="...">` | Ordered quickstarts and procedures (e.g. `get-started.mdx`). |
| `<CardGroup cols={2}>` / `<Card title icon href>` | Landing grids and "Next steps" link sets. Icons are duotone (`icon="duotone rocket"`); `href` is a slug path. |
| `<Tabs>` / `<Tab title="...">` | OS or language variants (Node vs. Python, macOS vs. Linux). |
| `<CodeBlocks>` | Grouping related multi-file or multi-language code samples. |
| `<Accordion>` / `<AccordionGroup>` | Collapsible secondary detail. Use sparingly. |
| `<EndpointRequestSnippet endpoint="GET /path" />` | Pull a live request example straight from the OpenAPI spec (`manual-authz.mdx`). Prefer this over hand-writing request samples. |

Code fences take a language and an optional title:
` ```bash `, ` ```yaml title="astropods.yml" `.

## Page structure

- Open with a one- or two-sentence lede stating what the page lets the reader do.
- Use `##` for major sections, `###` for subsections. Sentence case.
- Separate long sections with `---` when it aids scanning.
- **End task and concept pages with a `## Next steps`** section — either a bullet
  list of slug links or a `<CardGroup>`/`<Steps>` pointing to the natural
  follow-on pages. This is a consistent pattern across the tree.

## Links and cross-referencing

- Internal links are **URL paths**, not file paths: `[Install the CLI](/install-cli)`.
  For anything non-obvious, resolve the path with the `fern-docs` skill's link
  rules (walk `docs.yml` / read the target's `slug`) — don't guess from the
  filename.
- **One canonical home** per concept: the full explanation lives on one page;
  everything else links to it rather than restating it.
- Before adding a cross-reference, grep the tree for the feature name and read
  each hit:
  ```bash
  grep -rln "<feature>" docs-public/fern/docs/pages --include="*.mdx"
  ```
- **Lightest form first:** inline link inside an existing sentence → `<Note>`
  aside → new `##` section → new page.
- **Phrase inline links naturally.** Put the link on a noun phrase in the
  sentence ("counts against your [rate limit](...)"), not a tacked-on "See
  [page]." sentence.
- **Moving, renaming, or deleting a page changes its URL** — set up a redirect.
  Use the `fern-docs` skill's `references/redirects.md`.

## Changelog

Astro's changelog entries live in `fern/docs/changelog/` (wired in `docs.yml`
as `- changelog: "docs/changelog"`), one dated `.mdx` per entry.

- Frontmatter requires `tags`, e.g. `tags: ["release", "astro"]` or
  `["docs", "astro"]`. Reuse existing tags. `title` and `slug` are also permitted
  (Fern honors them; our entries use them) — set `title` when you want a headline
  distinct from the date.
- Lead each `##` feature with the user capability ("You can now…"), 2–6 sentences.
- For entry mechanics (filename date formats, tag filtering, RSS, the docs-link
  Button), defer to the `fern-docs` skill's `references/changelog.md`.

## LLM visibility tags

Fern supports tags that scope content by audience. Most content should stay
visible to both humans and agents.

- `<llms-only>` — content only agents see: extra step-by-step detail,
  prerequisite context, explicit cross-references. Keep standalone blocks
  self-contained.
- `<llms-ignore>` — content hidden from agents: decorative or marketing
  elements, internal comments.

## Commands

Run from `docs-public/fern`:

```bash
fern docs dev                    # local preview at http://localhost:3000
fern check                       # validate config, links, frontmatter
fern generate --docs --preview   # shareable preview build
```

CI runs `fern check` on every PR and publishes on merge to `main`
(`.github/workflows/fern-check.yml`, `fern-docs.yml`).
