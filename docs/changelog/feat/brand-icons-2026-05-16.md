# Brand icons package + agent-assisted sourcing tool

## Summary

Brand icons (the SVGs we serve to `astro-client` for integration tiles, agent cards, etc.) were previously a loose collection of files under `assets/integrations/light` and `assets/integrations/dark` with no source-of-truth, no provenance, no naming convention, and no story for adding new ones. This change moves them into a proper package (`@astropods/brand-icons`) with explicit canonical sources, a deterministic build step, and a developer tool for sourcing new icons through an LLM agent — under human review.

## Design

### One canonical source pair per icon, no derivation

The package's truth lives in `packages/astro-brand-icons/sources/`:

```
sources/<id>.svg        # renders correctly on a LIGHT background
sources/<id>.dark.svg   # renders correctly on a DARK background
```

There is no recoloring, no "mono vs. brand" mode, no `darkOverrides` map, no per-icon configuration. Every icon ships exactly two SVGs, both authored explicitly. The manifest (`icons.json`) is now just a list of ids — metadata is intentionally minimal so the source files are the contract.

The previous model recolored monochrome icons at build time and let "brand" icons pass through with optional dark-mode color overrides. It produced inconsistencies (some icons drifted between modes, some had hand-edited dark variants that the build couldn't reproduce) and offered no way to author per-variant differences cleanly. The 44 existing icons were migrated by capturing the current `assets/integrations/{light,dark}` outputs as the new canonical pair — the build now passes them through verbatim.

### Processor: normalize and emit, nothing else

`scripts/process.ts` reads every pair in `sources/`, normalizes for static delivery, and writes to `assets/integrations/{light,dark}/<id>.svg`:

- Strips `<title>`, `<desc>`, HTML comments.
- On the root `<svg>` tag only: drops intrinsic `width`/`height` and any inline `style` attribute. ViewBox + paths + xmlns is what stays. Consumers control sizing via CSS on their `<img>` — that's the modern icon convention.
- Everything else (artwork attributes, viewBox, preserveAspectRatio, etc.) is left untouched.

Paths resolve via `import.meta.url`, so the script works from any cwd. Defaults to `<repo>/assets/integrations`; `--out` and `--id` flags exist but the common case is `bun run process` with no args.

### Dev tool: Vite + React app at `packages/astro-brand-icons/`

`bun run dev` starts a single-page dev tool with two tabs that share state across navigation (both stay mounted; visibility toggled via CSS):

**Library tab** — searchable grid of every icon with a Light/Dark/Both toggle and a "Rebuild assets" button that POSTs to a Vite middleware which spawns the actual `scripts/process.ts`.

**Source tab** — a multi-turn chat with a Claude-backed agent. The agent's job is to find official brand SVGs and surface them as candidates the user reviews and saves.

### Agent: read-only, source-verbatim, candidates-only

The agent runs through the Claude Agent SDK with a strict contract:

- **Tools:** `WebSearch`, `WebFetch`, `Bash` (for downloading brand-kit zips), plus two custom MCP read-only tools — `list_icons` and `read_icon` — so the agent can see what's already in the package without being able to modify it.
- **Permission policy:** `canUseTool` callback denies any Bash command that references package paths (`sources/`, `icons.json`, `assets/integrations`, etc.). The agent's cwd is forced to `/tmp/astro-brand-icons-agent`. There is no `write_icon` tool — the agent has no way to mutate the library.
- **Editing is forbidden, not just discouraged.** The system prompt enumerates an explicit no-list: no recoloring fills, no trimming artwork, no path simplification, no "fixing" `currentColor`/`<style>` constructs, no synthesizing dark variants from light ones. When a fetched SVG fails the static-image rules, the instruction is to search for a different published asset, not repair it. Past behavior showed the model breaking fetched logos by "cleaning them up."
- **Multi-candidate, per-candidate id.** Each candidate carries its own kebab-case `id` and `brand`, so prompts like "find 4 brand icons we don't have yet" return 4 independently-saveable candidates with 4 different ids.
- **Conversation resumes via the SDK's `resume` option** using the session id returned on each turn — the chat is genuinely stateful, not just a re-played transcript.

### Save flow

Saving a candidate writes `sources/<id>.svg` + `sources/<id>.dark.svg`, upserts the manifest, and spawns the processor for that id. The Library refresh key bumps in the background so the grid is current the moment the user navigates back; the Source tab stays put with an in-place "✓ Saved <id>" notice (no redirect). To keep that flow non-disruptive, the Vite watcher ignores `sources/**`, `icons.json`, and `assets/integrations/**` — those writes would otherwise trigger an HMR full reload and lose the chat.

## Migration

Nothing to do for consumers of `assets/integrations/{light,dark}/<id>.svg` — the same files are still there, with cleaner root attributes (no intrinsic width/height, no inline styles on the root svg tag). Sizing should already be controlled by whoever embeds the `<img>`.

To add or modify icons going forward:

- **Manual:** drop `<id>.svg` and `<id>.dark.svg` into `packages/astro-brand-icons/sources/`, add `{ "id": "<id>" }` to `icons.json`, run `bun run process` (or click "Rebuild assets" in the dev tool).
- **Via the dev tool:** `bun run dev` in the package, use the Source tab.
