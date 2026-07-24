# Documentation style guide and compliance pass

## Summary

Product documentation had drifted in voice, structure, and accuracy: pages leaked internal service and infrastructure names, used inconsistent components and link styles, skipped prerequisites and verification steps, and in one case shipped unmodified starter-template boilerplate. This change brings every product page into line with the house style and tightens the audience rule in particular — no internal service, infra, or component names.

## Design

**House style.** The style guide lives in `docs-public/AGENTS.md` (from the authoring-skills work) and is checked with the `astro-docs-*` skills; `agents.md` / `CLAUDE.md` point at it. This pass applies it across the docs and reinforces one dimension in particular: write for the reader, and never expose internal service, infra, or component names.

**Compliance pass.** Applied the guide across the docs:

- **Audience.** Replaced internal names with reader-facing terms (observability vendor → "the trace data / Monitor view"; gateway internals → "the gateway"; the API service name → "the platform"; Kubernetes/pod/sidecar topology → observable behavior).
- **Consistency.** Normalized callouts to `<Note>` / `<Warning>`, made internal links root-relative to their slug, and standardized product naming ("Astro AI").
- **Task structure.** Added "Before you start" prerequisites, wrapped procedures in `<Steps>`, and ended setup flows with a verification step; added `<Warning>`s to opt-ins that change data collection or risk.
- **Accuracy.** Fixed a spec example that contradicted its own rules, corrected broken internal links, repaired a garbled callout and command block, rewrote the support page (previously starter boilerplate) to point at the real support channels, and corrected the frontend-agent authentication docs (the built-in OIDC sign-in is on by default).
- Renamed `private-database.mdx` to `ip-allowlisting.mdx` to match its published slug.

## Migration

None. Documentation only; page slugs are unchanged, so existing links continue to resolve.
