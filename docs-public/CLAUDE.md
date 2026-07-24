# docs-public

Astro's public product documentation — a [Fern](https://buildwithfern.com) site.

**When authoring or editing docs anywhere under `docs-public/`, follow
[`./AGENTS.md`](./AGENTS.md).** It defines the house style: voice, page
structure, frontmatter contract, component palette, and cross-referencing rules.

Tooling:
- Use the **`fern-docs`** skill for Fern mechanics — internal links, redirects,
  changelog wiring, product switchers, snippets, translations. It is a
  third-party skill, not vendored in this repo — install it once before editing:
  `npx skills add fern-api/skills --skill fern-docs -a claude-code` (see
  [`.claude/skills/README.md`](../.claude/skills/README.md)).
- Use **`astro-docs-style`** to check copy against the word/term list.
- Use **`astro-docs-review`** to QA a page (`fern check` + frontmatter + style +
  structure).
- Use **`astro-audit-cli-docs`** to check `cli-reference.mdx` against the `ast`
  CLI.

Preview and validate from `docs-public/fern`:

```bash
fern docs dev     # http://localhost:3000
fern check        # validate before opening a PR
```
