---
branch: sohum/unfurling
date: 2026-04-28
---

# Blueprint Link Preview (Social Unfurl)

## Summary

Blueprint pages previously emitted a generic account avatar as `og:image`, so every blueprint owned by the same account produced the same unfurl card. This adds a blueprint-specific 630×630 PNG rendered on demand by the Bun server, so each public blueprint unfurls with its own image in Slack, X, and LinkedIn. Private and draft blueprints produce no OG image.

## Design

### Badge endpoint

`GET /badge/agents/:account/:name.png` is intercepted in `server.ts` before the Go proxy and before React Router SSR. The Go server is never involved.

Request flow:
1. Validate account/name (regex + length bounds) — 404 on bad input.
2. Check in-memory cache — return immediately on hit.
3. Fetch blueprint from `API_URL/api/v1/agents/:account/:name`.
4. Gate on `visibility === "public"` and `versions.length > 0` — 404 with 5-minute negative cache otherwise.
5. Resolve agent avatar to a data URI.
6. Render SVG → rasterize with Resvg → cache → return PNG.

### blueprint-jellybean package

`apps/astro-client/blueprint-jellybean/` is a self-contained rendering package:

- **`card.ts`** — `buildBlueprintBadgeSvg()` produces a 630×630 SVG. Agent avatar centered (480×480, rounded corners) on a dark `#0a1614` background with a blueprint grid (20px, 0.5px lines, 25% opacity) and a radial teal glow. Thin accent banner at top. Astro AI wordmark (path-based, no font rendering) at bottom-left in teal + white. Avatar colors come from `avatar_colors` on the blueprint; falls back to default teal palette.

- **`assets.ts`** — `resolveAvatar` reads directly from `build/client/` to skip an HTTP round-trip; falls back to HTTP for CDN URLs. Handles SVG-in-JPEG avatars by rasterizing via Resvg; converts `oklch()` colors to hex before rasterizing (resvg doesn't support oklch).

- **`index.ts`** — HTTP handler, in-memory cache (`Map<string, Buffer>`), and Resvg orchestration. Fonts bundled: Inter-Regular, InterDisplay-SemiBold, GeistMono-Regular.

### Dead Go badge code removed

`handlers/badge.go`, `internal/badge/cache.go`, `internal/badge/render.go`, and the `golang-lru/v2` dependency were deleted. The Bun server intercepts `/badge/agents/` before anything reaches Go.

### OG tag changes (`BlueprintDetail.tsx`)

`og:image` points to `${origin}/badge/agents/{account}/{name}.png`. Tags are omitted entirely for private and draft blueprints. Canvas dimensions: `og:image:width: 630`, `og:image:height: 630`.

### Reverse proxy protocol fix (`server.ts`)

Loaders use `request.url` to build the `og:image` absolute URL. Behind cloudflared or a load balancer, the Bun-side connection is HTTP even though the public URL is HTTPS. Added `x-forwarded-proto` rewriting at the top of the fetch handler so all loaders see the correct protocol.

### Crawler streaming fix (`server.ts`)

React Router uses streaming SSR. Social crawlers (Slackbot, Facebookbot, Twitterbot, etc.) wait for the stream to close before parsing OG tags, causing timeouts and empty unfurls. When a crawler UA is detected, the full SSR response is buffered before being returned.

## Migration

No action required.
