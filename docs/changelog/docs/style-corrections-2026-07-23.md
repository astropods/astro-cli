# Bring docs-public inline with the authoring conventions

## Summary

Running the new docs-authoring skills (`astro-audit-cli-docs`, `astro-docs-style`,
`astro-docs-review`) against `docs-public/` surfaced drift from the house
conventions: CLI-reference gaps, brand/spelling/voice violations, internal links
written as file paths (two of them broken), guide pages missing a closing
`## Next steps`, and stale Fern starter-template pages. This applies the
corrections so the published docs match `docs-public/AGENTS.md`.

## Design

Corrections grouped by category:

- **CLI reference (`cli-reference.mdx`).** Documented the missing visible commands
  `blueprint set` and `project trigger`, and the missing flags `--wait`
  (`blueprint deploy`/`agent redeploy`), `--template` (`blueprint get`), `-y/--yes`
  and `-V` (`blueprint push`), `--all-logs` (`project start`), and the `mcp`
  template (`project create`). Fixed the stale instruction pointing at the
  nonexistent `ast add`. The reference is scoped to the **prod `ast`** surface, so
  `ast knowledge`, the top-level aliases, hidden commands, and the dev-build-only
  `account token` are intentionally excluded. The internal spec
  (`docs/02-cli/cli-command-tree.md`) now marks hidden commands and drops `add`.
- **Voice and terminology.** Brand fix (`AstroAI` → `Astropods`), US spelling
  (`normalise`/`flavours` → `normalize`/`flavors`, `cancelled` → `canceled`),
  removed hedges (`just`, `simply`), rephrased capability voice (`lets/allows
  you`), adopted the house click-path form (`**A** > **B**`), and replaced
  reader-facing `via`. Two page titles moved to sentence case.
- **Links.** Converted file-path internal links (`./x`, `../x`, `/docs/pages/x`)
  to slug paths across the adapters and messaging-SDK pages; two were broken
  targets (`./astropods-package-spec` resolved to a nonexistent nested path).
- **Structure.** Added a closing `## Next steps` to the guide/task pages that
  lacked one, and reordered `get-started` so Next steps closes the page.
- **Orphans.** Deleted five Fern starter-template pages (`writing-content`,
  `editing-your-docs`, `navigation`, `customization`, `support`) and the
  duplicate `publish-to-registry` (its install-cli link now points at the
  blueprints guide). `api-reference-overview` stays out of nav — the API tab is
  generated from the OpenAPI spec.

The RFC/spec pages (`astropods-package-spec`, `agent-card-spec`) keep their
Title-Case numbered headings under the exemption added to `AGENTS.md`.

## Migration

Redirects were added in `docs.yml` for every removed page: `/publish-to-registry`
→ `/blueprints`, and the five Fern-starter slugs → `/welcome`. No action required
for readers; existing links continue to resolve.
