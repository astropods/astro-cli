# Claude Code skills

Project skills for working on Astro. Our own skills are tracked in git; the
third-party `fern-docs` skill is installed on demand and is **not** committed.

## Our skills (tracked)

| Skill | Purpose |
|-------|---------|
| `docs-map` | Look up which `docs/` (internal spec/architecture/guide/plan) files are canonical for a feature area, and check them after a change. Not for `docs-public/`. |
| `write-changelog` | Write or update the `docs/changelog/` entry required on every PR. |
| `write-release-notes` | Cut a new `docs/releases/` note and tag it, per `agents.md`'s Releases process. |
| `astro-docs-style` | Check `docs-public/` copy against the house word/term list; answer style questions. |
| `astro-docs-review` | Per-page QA for `docs-public/`: `fern check` + frontmatter + style + structure. |
| `astro-audit-cli-docs` | Audit the public CLI reference against the prod `ast` CLI surface. |

## fern-docs (third-party, install before editing docs)

`fern-docs` comes from [`fern-api/skills`](https://github.com/fern-api/skills)
and covers generic Fern mechanics (internal links, redirects, changelog wiring,
product switchers, snippets, translations). It is **not vendored** — it has its
own upstream, license, and update cadence, and runs with full agent permissions.

Install it before editing `docs-public/`:

```bash
npx skills add fern-api/skills --skill fern-docs -a claude-code
```

It installs to `.claude/skills/fern-docs/` (git-ignored). Re-run to update.
Optionally connect Fern's live MCP for syntax lookups:

```bash
claude mcp add --transport http fern https://buildwithfern.com/learn/_mcp/server
```
