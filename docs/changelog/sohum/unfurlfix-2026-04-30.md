---
branch: sohum/unfurlfix
date: 2026-04-30
---

## Summary

Two bugs prevented agent badge cards from rendering correctly in Slack unfurls: the badge image rendered with no text, and the `og:image` URL used `http://` instead of `https://`, causing Slack to fall back to a small thumbnail instead of the full-width preview card.

## Design

**Blank badge text (font path broken in SSR build)**

`blueprint-jellybean` resolves font files at runtime using `path.dirname(fileURLToPath(import.meta.url))`. When Vite bundles the package for SSR, `import.meta.url` resolves to the server bundle file (`build/server/assets/server-build-*.js`), not the original source package. The `fonts/` directory didn't exist relative to the bundle, so Resvg loaded no fonts and silently dropped all text elements.

Fix: a `copyBlueprintFonts` Vite plugin (in `vite.config.ts`) copies the font files into `{build.outDir}/server/assets/fonts/` after each build. The destination is derived from `config.build.outDir` via the `configResolved` hook — no hardcoded paths.

**`http://` OG image URLs (behind Cloudfront)**

The `BlueprintDetail` loader builds the `og:image` URL using `process.env.PUBLIC_ORIGIN || new URL(request.url).origin`. Behind Cloudfront, the request arrives at the Node.js origin over HTTP, so the fallback produces `http://astropod.ai/...`. Slack requires HTTPS for `summary_large_image`.

Fix: the loader now also checks `process.env.FRONTEND_DOMAIN` (already present in the `astro-client-config` cluster ConfigMap), prepending `https://` so the OG URL is always secure.

## Migration

No action required for users. Ops: confirm `FRONTEND_DOMAIN` is set in the `astro-client-config` ConfigMap (it should already be).
