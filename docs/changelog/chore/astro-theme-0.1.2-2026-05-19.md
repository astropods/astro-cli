## Summary

Bump `@astropods/theme` to `0.1.2` and publish. The published `0.1.1` predates several theme changes that have accumulated in `main`, including the `slate` palette added in the design-system overhaul. Downstream consumers (notably `modules/playground`) reference `var(--color-slate-*)` and `bg-slate-*` utilities that resolve to nothing against the published artifact, breaking light/dark surfaces.

## Design

No source changes — the version bump alone triggers `.github/workflows/publish-theme.yml` on merge to `main`. The workflow reads `package.json`, checks the registry for the new version, builds via `moon run astro-theme:build`, and runs `npm publish` from `packages/astro-theme/` with OIDC-based trusted publishing.

Unreleased commits that ship in `0.1.2`:

- `852ebeb4` color system overhaul + dark mode cleanup (adds `slate`)
- `7d1276ad` shift teal palette to muted slate-teal
- `745566f4` dark mode polish and flash-of-light-mode fix
- `aa1325de` theme sync with tests
- `1ca91706`, `906ec744`, `a6030766` knowledge / status-badge polish that touched semantic tokens

## Migration

Consumers pinned to `^0.1.1` will pick up `0.1.2` on next `bun install`. Submodules that pin explicitly (e.g. `modules/playground` in its own repo) must bump their `package.json` reference to `^0.1.2` in a separate PR once this publish lands.
