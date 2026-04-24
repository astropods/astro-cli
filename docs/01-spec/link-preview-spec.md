# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

When a user pastes an Astro agent URL into Slack, LinkedIn, X (Twitter), or any Open Graph-aware platform, the platform crawls that URL and renders an unfurl card — a rich preview with a title, description, and image. Currently, Astro pages return no Open Graph metadata, so pasted links appear as plain text. This spec defines how to add dynamic link previews to agent and blueprint pages by generating server-side PNG images from the existing `astro-trading-card` package and injecting per-page Open Graph meta tags via React Router's `meta` export.

---

## Problem Statement

Sharing an Astro agent link today produces no visual preview on any social or messaging platform. This is a missed distribution opportunity: every shared link is a chance to showcase the agent's identity, integrations, and capabilities. The root causes are:

1. **No `og:image`** — Astro pages return no Open Graph meta tags of any kind.
2. **SVG can't serve as OG image** — Platforms like LinkedIn, X, and Slack require a raster image (PNG or JPEG) at the `og:image` URL. SVGs are either ignored or blocked.
3. **No server-side PNG generation** — The `astro-trading-card` package generates SVG strings only. PNG export currently requires a browser canvas, which cannot be invoked during an HTTP request.

---

## Goals

- **G1:** When any agent detail page URL is pasted into Slack, LinkedIn, or X, an unfurl card renders with the agent's trading card as the preview image.
- **G2:** The PNG image is generated server-side on demand with no browser involved.
- **G3:** OG tags are populated with accurate, agent-specific data (not generic site-wide defaults).
- **G4:** The PNG endpoint is cacheable at the HTTP layer to avoid repeated re-renders.
- **G5:** The implementation does not require changes to the Go backend.

## Non-Goals

- Blueprint page unfurling (can be added in a follow-up using the same pattern).
- Animated or video preview cards.
- Uploading pre-generated PNGs to S3/CDN (can be layered on later as a caching optimization).
- Per-user or per-session personalization of the card image.
- Changing the visual design of the trading card itself.

---

## Design

### Overview

```
[Social platform crawler]
         │  GET /agents/:account/:name
         ▼
[Bun SSR server]  ──────────────────────────────────────────────────────────┐
   React Router SSR                                                           │
   meta() export returns:                                                     │
     og:title    = agent display name                                         │
     og:description = agent description                                       │
     og:image    = https://<host>/badge/:account/:name.png  ◄────────────────┘
     twitter:card = summary_large_image
         │
         │  (crawler fetches og:image URL)
         ▼
[Bun SSR server]  GET /badge/:account/:name.png
   1. Fetch agent data from Go API  (GET /api/v1/agents/:account/:name)
   2. Map agent data → CardData
   3. generateCard(cardData)         ← astro-trading-card (SVG string)
   4. Resvg(svg).render()           ← @resvg/resvg-js (PNG buffer)
   5. Return image/png + Cache-Control: public, max-age=3600
```

The Bun server (`server.ts`) is the right place to handle `/badge/*` — it already sits in front of React Router and proxies selectively to the Go backend. Adding a badge route here requires no changes to Go and keeps image generation in the same runtime that owns the `astro-trading-card` package.

---

### 1. Badge PNG Endpoint

**Route:** `GET /badge/:account/:name.png`

This route MUST be handled by the Bun server before the React Router handler runs. It is not proxied to the Go backend.

#### Request

| Parameter | Location | Description |
|-----------|----------|-------------|
| `account` | Path | Account/org slug (e.g., `postman`) |
| `name` | Path | Agent slug (e.g., `research-assistant`) |

#### Response

| Scenario | Status | Body |
|----------|--------|------|
| Success | `200 OK` | PNG binary, `Content-Type: image/png` |
| Agent not found | `404 Not Found` | Empty body |
| Upstream API error | `502 Bad Gateway` | Empty body |

**Headers on success:**
```
Content-Type: image/png
Cache-Control: public, max-age=3600, stale-while-revalidate=86400
```

#### Generation Flow

```typescript
// server.ts (new handler, before React Router)

import { generateCard } from "@postman/astro-trading-card";
import { Resvg } from "@resvg/resvg-js";

async function handleBadgeRequest(account: string, name: string): Promise<Response> {
  // 1. Fetch agent data
  const res = await fetch(`${API_URL}/api/v1/agents/${account}/${name}`);
  if (!res.ok) return new Response(null, { status: res.status === 404 ? 404 : 502 });
  const agent = await res.json();

  // 2. Map to CardData
  const cardData = agentToCardData(agent);

  // 3. Generate SVG
  const svg = generateCard(cardData);

  // 4. Render PNG
  const resvg = new Resvg(svg, { background: "rgba(0,0,0,0)" });
  const png = resvg.render().asPng();

  // 5. Respond
  return new Response(png, {
    headers: {
      "Content-Type": "image/png",
      "Cache-Control": "public, max-age=3600, stale-while-revalidate=86400",
    },
  });
}
```

---

### 2. Agent Data → CardData Mapping

The `agentToCardData()` function MUST map the Go API agent response to `CardData` from `astro-trading-card`. The exact shape of the API response should be confirmed against `/api/v1/agents/:account/:name`, but the expected mapping is:

| CardData field | Source | Notes |
|---------------|--------|-------|
| `name` | `agent.name` | Slug, used for barcode |
| `displayName` | `agent.display_name` | Falls back to `agent.name` |
| `account` | `agent.account` | Org/account slug |
| `description` | `agent.description` | Truncated to ~120 chars if needed |
| `avatar` | `agent.avatar_url` | `{ url: agent.avatar_url }` if present |
| `stats[0]` | `agent.deployment_count` | Label: `"Deployments"`, value: count as string |
| `stats[1]` | `agent.created_at` | Label: `"Created"`, value: formatted date |
| `integrations` | `agent.integrations[]` | Map name + icon to `CardIntegration` |
| `qrUrl` | `https://<host>/agents/:account/:name` | Public URL for QR code |
| `barcodeId` | `agent.name` | Used for Code 128B barcode |

Colors are not specified — the card SHOULD use `deriveCardColors()` from the package if a dominant avatar color is available, otherwise fall back to `DEFAULT_COLORS`.

---

### 3. Open Graph Meta Tags

React Router v7 supports per-route `meta()` exports that run during SSR. The agent detail route MUST export a `meta` function that returns the following tags.

**Tags to add:**

| Property | Value |
|----------|-------|
| `og:type` | `website` |
| `og:title` | `{agent.display_name} — Astro` |
| `og:description` | `{agent.description}` (truncated to 200 chars) |
| `og:image` | `https://{host}/badge/{account}/{name}.png` |
| `og:image:width` | `350` |
| `og:image:height` | `560` |
| `og:url` | Canonical page URL |
| `twitter:card` | `summary_large_image` |
| `twitter:title` | Same as `og:title` |
| `twitter:description` | Same as `og:description` |
| `twitter:image` | Same as `og:image` |

The `host` MUST be derived from the incoming request's `Host` header (already available in React Router loaders), not hardcoded, to work across environments (local, staging, production).

**Example meta export:**

```typescript
// DeployedAgentDetail.tsx (or its route module)
export const meta: MetaFunction<typeof loader> = ({ data, location }) => {
  if (!data?.agent) return [];
  const { agent, host } = data;
  const imageUrl = `https://${host}/badge/${agent.account}/${agent.name}.png`;
  return [
    { property: "og:type", content: "website" },
    { property: "og:title", content: `${agent.display_name} — Astro` },
    { property: "og:description", content: agent.description?.slice(0, 200) ?? "" },
    { property: "og:image", content: imageUrl },
    { property: "og:image:width", content: "350" },
    { property: "og:image:height", content: "560" },
    { property: "og:url", content: `https://${host}${location.pathname}` },
    { name: "twitter:card", content: "summary_large_image" },
    { name: "twitter:title", content: `${agent.display_name} — Astro` },
    { name: "twitter:description", content: agent.description?.slice(0, 200) ?? "" },
    { name: "twitter:image", content: imageUrl },
  ];
};
```

The loader MUST include `host` in its returned data. It can be extracted from the React Router `request` object: `new URL(request.url).host`.

---

### 4. In-Memory Cache

To avoid re-rendering the SVG → PNG on every crawl (multiple crawlers hit the same URL during unfurl), the Bun server SHOULD maintain a simple in-memory LRU cache keyed by `account/name`.

| Property | Value |
|----------|-------|
| Cache key | `${account}/${name}` |
| Cached value | PNG `Buffer` |
| Max entries | 500 |
| TTL | 1 hour (3600s) |
| Eviction | LRU |

This cache is process-local and does not survive restarts. Persistent caching (S3, Redis) can be layered on as a follow-up. The HTTP `Cache-Control` header handles CDN-level caching independently.

A lightweight LRU implementation (e.g., `lru-cache` npm package, ~2KB) is preferred over a hand-rolled map to avoid unbounded memory growth.

---

### 5. Dependency Changes

| Package | Location | Change |
|---------|----------|--------|
| `@resvg/resvg-js` | `apps/astro-client` | Add as dependency |
| `lru-cache` | `apps/astro-client` | Add as dependency |
| `@postman/astro-trading-card` | `apps/astro-client` | Already present (confirm version) |

`@resvg/resvg-js` ships a prebuilt native binary for each platform. It works with Bun's Node-compatible native addon loader. No WASM fallback is needed since the Bun server runs in a controlled server environment (not an edge runtime).

---

## Files to Modify or Create

| File | Change |
|------|--------|
| `apps/astro-client/server.ts` | Add `/badge/:account/:name.png` route before React Router handler; import Resvg and LRU cache |
| `apps/astro-client/src/badge.ts` | **New file.** `agentToCardData()` mapping function and PNG generation helper |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add `meta()` export with OG tags; add `host` to loader return value |
| `apps/astro-client/package.json` | Add `@resvg/resvg-js`, `lru-cache` |

---

## Implementation Order

**Phase 1 — PNG endpoint (no visible user change):**
1. Install `@resvg/resvg-js` and `lru-cache`
2. Create `src/badge.ts` with `agentToCardData()` and `generateBadgePng()`
3. Add `/badge/:account/:name.png` handler in `server.ts`
4. Verify locally: `curl http://localhost:3000/badge/postman/research-assistant.png > card.png`

**Phase 2 — OG meta tags (enables unfurling):**
1. Add `host` to the agent detail loader
2. Add `meta()` export to `DeployedAgentDetail.tsx`
3. Verify using [LinkedIn Post Inspector](https://www.linkedin.com/post-inspector/) and [Twitter Card Validator](https://cards-dev.twitter.com/validator) against a staging URL
4. Verify Slack unfurl by pasting URL into a test channel

These phases can ship independently. Phase 1 is purely additive and carries no user-visible risk. Phase 2 only affects crawlers until users start sharing links.

---

## Key Design Decisions

**1. PNG generation in the Bun server, not the Go backend.**  
The `astro-trading-card` package is TypeScript. Calling it from Go would require spawning a subprocess or duplicating the card layout logic in Go — both are poor options. The Bun server already runs TypeScript and is the natural owner of this work. The Go backend is not modified.

**2. On-demand generation, not pre-generation.**  
Pre-generating PNGs at agent creation time would require the backend to know about the Bun server (or a separate image service), adding coupling. On-demand generation is simpler and the LRU + HTTP cache provides sufficient performance for crawlers (which are low-frequency compared to human traffic).

**3. Resvg over browser canvas.**  
`@resvg/resvg-js` is the same library used by `flows-jellybean` in this monorepo — it is a proven, Rust-based SVG renderer that produces consistent, high-quality output. Browser canvas rasterization (via `downloadPng()` in `astro-trading-card`) requires a headless browser (Puppeteer/Playwright), which is a heavy server dependency and slow cold-start.

**4. `summary_large_image` Twitter card type.**  
The trading card is 350×560px — taller than wide. This is not ideal for the `summary_large_image` format (which renders best at ~2:1 wide). Teams should evaluate whether to: (a) accept the letterboxed rendering, (b) create a landscape card variant (e.g., 600×315px), or (c) use `summary` card type with a square crop. This decision is left for design review.

**5. No OG tags in `root.tsx`.**  
Global fallback OG tags (e.g., a generic Astro site image) are intentionally omitted from `root.tsx`. Adding them would cause non-agent pages to unfurl with incorrect metadata. Per-page `meta()` exports are the right scope.
