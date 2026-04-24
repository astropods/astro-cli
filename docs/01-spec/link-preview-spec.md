# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

When a user pastes an Astro URL into Slack, LinkedIn, X (Twitter), or any Open Graph-aware platform, the platform crawls that URL and renders an unfurl card — a rich preview with a title, description, and image. Blueprint pages already emit Open Graph metadata, but the preview image is the **account's profile avatar** — a generic photo with no connection to the blueprint itself. Agent detail pages have no OG metadata at all. This spec upgrades both surfaces by generating server-side PNG images that are specific to each entity and injecting or replacing the relevant Open Graph meta tags.

The two page types use distinct image styles and rendering approaches, reflecting their different roles in the product:

- **Agent pages** use the existing `astro-trading-card` package to produce a portrait trading card (dark, holographic-styled), rendered to PNG via Resvg.
- **Blueprint pages** upgrade the existing `og:image` from the account avatar to a new Satori-rendered card that mirrors the blueprint listing UI — a clean, light landscape card with icon, name, description, deploy count, and owner.

---

## Problem Statement

Sharing an Astro link today produces a weak or absent unfurl depending on the page type:

- **Blueprint pages** (`BlueprintDetail.tsx`) already emit `og:title`, `og:description`, `og:image`, and `twitter:card` tags. However, the `og:image` is set to `${assetsBase}/avatars/${account}.jpg` — the account's profile photo. This is generic: every blueprint owned by the same account shows the same image, with no visual information about the blueprint itself.
- **Agent detail pages** emit no OG metadata whatsoever. Pasted agent URLs appear as plain text on all platforms.

The core technical gap is the same in both cases: there is no server-side mechanism to generate a PNG image that represents a specific entity. The `astro-trading-card` package produces SVG strings, and SVG cannot be served as an `og:image` — platforms like LinkedIn, X, and Slack require a raster image (PNG or JPEG) at that URL.

---

## Goals

- **G1:** Agent detail page URLs unfurl with a trading card PNG as the preview image.
- **G2:** Blueprint page URLs replace the current account-avatar `og:image` with a UI-style landscape card PNG specific to the blueprint.
- **G3:** All PNG images are generated server-side on demand with no browser involved.
- **G4:** OG tags are populated with accurate, entity-specific data.
- **G5:** PNG endpoints are cacheable at the HTTP layer to avoid repeated re-renders.
- **G6:** The implementation does not require changes to the Go backend.

## Non-Goals

- Animated or video preview cards.
- Uploading pre-generated PNGs to S3/CDN (can be layered on later as a caching optimization).
- Per-user or per-session personalization of the card image.
- Changing the visual design of the trading card itself.

---

## Design

### Overview

There are two independent badge routes, one per page type. Both are handled by the Bun server before the React Router handler runs and neither is proxied to the Go backend.

```
[Social platform crawler]
         │  GET /:account/:name          (agent page)
         │  GET /blueprints/:account/:name   (blueprint page)
         ▼
[Bun SSR server — React Router SSR]
   meta() export returns og:image pointing at badge route
         │
         ├─► GET /badge/agent/:account/:name.png
         │       1. Fetch agent from API
         │       2. agentToCardData()
         │       3. generateCard()          ← astro-trading-card
         │       4. Resvg().render()        ← @resvg/resvg-js
         │       5. Return PNG
         │
         └─► GET /badge/blueprint/:account/:name.png
                 1. Fetch blueprint from API
                 2. blueprintToCardProps()
                 3. satori(<BlueprintCard />)  ← satori (JSX → SVG)
                 4. Resvg().render()           ← @resvg/resvg-js
                 5. Return PNG
```

---

### 1. Agent Badge Endpoint

**Route:** `GET /badge/agent/:account/:name.png`

Renders the agent's `astro-trading-card` as a PNG. Portrait format (350×560px).

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
import { generateCard } from "@postman/astro-trading-card";
import { Resvg } from "@resvg/resvg-js";

async function handleAgentBadge(account: string, name: string): Promise<Response> {
  const res = await fetch(`${API_URL}/api/v1/agents/${account}/${name}`);
  if (!res.ok) return new Response(null, { status: res.status === 404 ? 404 : 502 });

  const agent = await res.json();
  const svg = generateCard(agentToCardData(agent));
  const png = new Resvg(svg).render().asPng();

  return new Response(png, {
    headers: {
      "Content-Type": "image/png",
      "Cache-Control": "public, max-age=3600, stale-while-revalidate=86400",
    },
  });
}
```

#### Agent Data → CardData Mapping

| CardData field | Source | Notes |
|---------------|--------|-------|
| `name` | `agent.name` | Slug, used for barcode |
| `displayName` | `agent.display_name` | Falls back to `agent.name` |
| `account` | `agent.account` | Org/account slug |
| `description` | `agent.description` | Truncated to ~120 chars |
| `avatar` | `agent.avatar_url` | `{ url: agent.avatar_url }` if present |
| `stats[0]` | `agent.deployment_count` | Label: `"Deployments"` |
| `stats[1]` | `agent.created_at` | Label: `"Created"`, formatted date |
| `integrations` | `agent.integrations[]` | Mapped to `CardIntegration[]` |
| `qrUrl` | Canonical agent URL | For QR code in barcode footer |
| `barcodeId` | `agent.name` | For Code 128B barcode |

Colors SHOULD use `deriveCardColors()` if a dominant avatar color is available, otherwise fall back to `DEFAULT_COLORS`.

---

### 2. Blueprint Badge Endpoint

**Route:** `GET /badge/blueprint/:account/:name.png`

Renders a landscape card that mirrors the blueprint listing UI. This format is natively landscape (~1200×630px) — the standard OG image aspect ratio — which avoids the letterboxing issue that the portrait trading card has on X/Twitter.

#### Visual Design

The blueprint card matches the blueprint listing UI card design:

```
┌─────────────────────────────────────────────────────────┐  ← light gray bg
│ · · · · · · · · · · · · · · · · · · · · · · · · · · · · │  ← dot grid pattern
│  ┌──────┐  release-note-helper                          │
│  │  @@  │  An agent that helps you craft release notes  │
│  └──────┘  from Jira issues and GitHub PRs              │
│ · · · · · · · · · · · · · · · · · · · · · · · · · · · · │
│  ─────────────────────────────────────────────────────  │  ← divider
│  0 deploys                        ◉ sohumdalal          │
└─────────────────────────────────────────────────────────┘
```

| Property | Value |
|----------|-------|
| Dimensions | 1200×630px |
| Background | Light gray (`#f8f9fa`) with dot grid overlay |
| Icon | Rounded square, 80×80px, agent avatar or initials fallback |
| Title | Bold, ~28px, dark gray (`#111827`) |
| Description | Regular, ~18px, medium gray (`#6b7280`), max 2 lines |
| Divider | 1px line, light gray (`#e5e7eb`) |
| Footer left | Deploy count, ~16px, medium gray |
| Footer right | Owner avatar (24px circle) + handle, ~16px |
| Corner radius | 12px |
| Border | 1px, `#e5e7eb` |

#### Rendering Approach: Satori

Blueprint cards are rendered with **Satori** (`satori` npm package), which converts a JSX component tree to an SVG string using flexbox layout — the same technology behind `@vercel/og`. The SVG is then passed to Resvg for PNG conversion, the same final step as the agent badge.

Satori is preferred here over a hand-written SVG template because:
1. The blueprint card's layout maps naturally to flexbox (which Satori supports natively).
2. A JSX component can be visually maintained alongside the equivalent UI component, keeping them in sync.
3. Satori handles text wrapping, line clamping, and image embedding — all required for this card.

```typescript
import satori from "satori";
import { Resvg } from "@resvg/resvg-js";
import { readFileSync } from "fs";

const font = readFileSync("path/to/Inter-Regular.ttf");
const fontBold = readFileSync("path/to/Inter-Bold.ttf");

async function handleBlueprintBadge(account: string, name: string): Promise<Response> {
  const res = await fetch(`${API_URL}/api/v1/blueprints/${account}/${name}`);
  if (!res.ok) return new Response(null, { status: res.status === 404 ? 404 : 502 });

  const blueprint = await res.json();
  const props = blueprintToCardProps(blueprint);

  const svg = await satori(
    <BlueprintCard {...props} />,
    {
      width: 1200,
      height: 630,
      fonts: [
        { name: "Inter", data: font, weight: 400 },
        { name: "Inter", data: fontBold, weight: 700 },
      ],
    }
  );

  const png = new Resvg(svg).render().asPng();

  return new Response(png, {
    headers: {
      "Content-Type": "image/png",
      "Cache-Control": "public, max-age=3600, stale-while-revalidate=86400",
    },
  });
}
```

The `BlueprintCard` JSX component lives in `src/badge-blueprint.tsx` and is the source of truth for the card's visual design. It MUST NOT import any browser-only APIs, Tailwind classes, or CSS modules — Satori requires inline styles only.

#### Blueprint Data → CardProps Mapping

| CardProps field | Source | Notes |
|----------------|--------|-------|
| `name` | `blueprint.name` | Slug |
| `displayName` | `blueprint.display_name` | Falls back to `blueprint.name` |
| `description` | `blueprint.description` | Clamped to 2 lines in layout |
| `avatarUrl` | `blueprint.avatar_url` | Falls back to initials avatar |
| `deployCount` | `blueprint.deployment_count` | Displayed as `"{n} deploys"` |
| `ownerHandle` | `blueprint.account` | Displayed in footer right |
| `ownerAvatarUrl` | `blueprint.account_avatar_url` | Small circle in footer |

#### Font Loading

Satori requires font data to be loaded as `ArrayBuffer`. Fonts SHOULD be loaded once at server startup (not per-request) and stored in module scope. Inter (Regular 400, Bold 700) is the recommended typeface to match the UI.

---

### 3. Open Graph Meta Tags

React Router v7 supports per-route `meta()` exports that run during SSR. Both the agent detail route and the blueprint detail route MUST export a `meta` function.

The `host` MUST be derived from `new URL(request.url).host` in the loader and returned as part of loader data — never hardcoded — to work correctly across local, staging, and production environments.

#### Agent Detail Page

| Property | Value |
|----------|-------|
| `og:type` | `website` |
| `og:title` | `{agent.display_name} — Astro` |
| `og:description` | `{agent.description}` (max 200 chars) |
| `og:image` | `https://{host}/badge/agent/{account}/{name}.png` |
| `og:image:width` | `350` |
| `og:image:height` | `560` |
| `og:url` | Canonical agent page URL |
| `twitter:card` | `summary` (portrait card; see Design Decision #4) |
| `twitter:image` | Same as `og:image` |

#### Blueprint Detail Page

`BlueprintDetail.tsx` already exports a `meta()` function with `og:title`, `og:description`, `og:url`, `og:image`, and `twitter:card`. The only change required is replacing the `ogImage` value passed from the loader — currently `${assetsBase}/avatars/${account}.jpg` — with the new badge endpoint URL. All other tags remain unchanged.

| Property | Current value | New value |
|----------|--------------|-----------|
| `og:image` | `${assetsBase}/avatars/${account}.jpg` | `https://{host}/badge/blueprint/{account}/{name}.png` |
| `og:image:width` | _(not set)_ | `1200` |
| `og:image:height` | _(not set)_ | `630` |
| `twitter:image` | Same as `og:image` | Same as new `og:image` |
| All other tags | Unchanged | Unchanged |

The loader change is minimal: replace the `ogImage` derivation from the account avatar URL to the badge endpoint URL.

---

### 4. In-Memory Cache

Both badge endpoints SHOULD share a single LRU cache, keyed by the full route string.

| Property | Value |
|----------|-------|
| Cache key | `agent:${account}/${name}` or `blueprint:${account}/${name}` |
| Cached value | PNG `Uint8Array` |
| Max entries | 500 |
| TTL | 1 hour (3600s) |
| Eviction | LRU |

A lightweight LRU implementation (e.g., `lru-cache` npm package) is preferred over a hand-rolled map to avoid unbounded memory growth. This cache is process-local; persistent caching (S3, Redis) can be layered on as a follow-up. The HTTP `Cache-Control` header handles CDN-level caching independently.

---

### 5. Dependency Changes

| Package | Location | Change |
|---------|----------|--------|
| `@resvg/resvg-js` | `apps/astro-client` | Add as dependency |
| `satori` | `apps/astro-client` | Add as dependency |
| `lru-cache` | `apps/astro-client` | Add as dependency |
| `@postman/astro-trading-card` | `apps/astro-client` | Already present (confirm version) |

`@resvg/resvg-js` ships a prebuilt native binary for each platform and works with Bun's Node-compatible native addon loader. No WASM fallback is needed since the Bun server runs in a controlled server environment (not an edge runtime). The same applies to `satori`, which is pure JavaScript.

---

## Files to Modify or Create

| File | Change |
|------|--------|
| `apps/astro-client/server.ts` | Add `/badge/agent/*` and `/badge/blueprint/*` routes before React Router handler |
| `apps/astro-client/src/badge-agent.ts` | **New.** `agentToCardData()` and `generateAgentBadgePng()` |
| `apps/astro-client/src/badge-blueprint.tsx` | **New.** `BlueprintCard` JSX component (Satori-compatible, inline styles only) and `generateBlueprintBadgePng()` |
| `apps/astro-client/src/badge-cache.ts` | **New.** Shared LRU cache instance |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add `meta()` export with agent OG tags; add `host` to loader |
| `apps/astro-client/src/pages/blueprints/Blueprints.tsx` | Add `meta()` export with blueprint OG tags; add `host` to loader (confirm route file) |
| `apps/astro-client/package.json` | Add `@resvg/resvg-js`, `satori`, `lru-cache` |

---

## Implementation Order

**Phase 1 — Agent badge PNG endpoint:**
1. Install `@resvg/resvg-js` and `lru-cache`
2. Create `src/badge-agent.ts` and `src/badge-cache.ts`
3. Add `/badge/agent/:account/:name.png` handler in `server.ts`
4. Verify: `curl http://localhost:3000/badge/agent/postman/research-assistant.png > agent.png`

**Phase 2 — Agent OG meta tags:**
1. Add `host` to agent detail loader
2. Add `meta()` export to `DeployedAgentDetail.tsx`
3. Verify with LinkedIn Post Inspector and Twitter Card Validator against staging

**Phase 3 — Blueprint badge PNG endpoint:**
1. Install `satori`; confirm Inter font files are accessible server-side
2. Create `src/badge-blueprint.tsx` with `BlueprintCard` component
3. Add `/badge/blueprint/:account/:name.png` handler in `server.ts`
4. Verify: `curl http://localhost:3000/badge/blueprint/postman/release-note-helper.png > blueprint.png`

**Phase 4 — Blueprint OG meta tags:**
1. Add `host` to blueprint detail loader
2. Add `meta()` export to the blueprint detail route
3. Verify Slack unfurl by pasting a blueprint URL into a test channel

Phases ship independently. Phases 1 and 3 are purely additive with no user-visible change. Phases 2 and 4 activate unfurling as soon as they deploy.

---

## Key Design Decisions

**1. PNG generation in the Bun server, not the Go backend.**  
The `astro-trading-card` package and Satori are both TypeScript libraries. Calling them from Go would require spawning a subprocess or duplicating layout logic — both are poor options. The Bun server already runs TypeScript and is the natural owner of this work. The Go backend is not modified.

**2. Different rendering stacks for agents vs. blueprints.**  
Agent cards reuse the existing `astro-trading-card` package (which already produces polished SVG output) plus Resvg. Blueprint cards use Satori because the target visual is the existing UI card component — JSX with inline styles is the most maintainable way to reproduce and keep that in sync. A single approach for both would require either forcing the UI-style card into a hand-written SVG template (brittle) or rebuilding the trading card in Satori (unnecessary work).

**3. On-demand generation, not pre-generation.**  
Pre-generating PNGs at creation time would require the backend to know about the Bun server (or a separate image service), adding coupling. On-demand generation is simpler, and the LRU + HTTP cache provides sufficient performance for crawlers, which are low-frequency compared to regular user traffic.

**4. Twitter card type: `summary` for agents, `summary_large_image` for blueprints.**  
The trading card is 350×560px (portrait). `summary_large_image` expects a ~2:1 landscape image and letterboxes portrait images poorly on X. Using `twitter:card = summary` renders it as a square thumbnail instead, which is a better fit. The blueprint card is 1200×630px (native OG landscape), so `summary_large_image` is correct there. If a landscape agent card variant is designed in the future, the agent card type can be upgraded to `summary_large_image`.

**5. No OG tags in `root.tsx`.**  
Global fallback OG tags are intentionally omitted from `root.tsx`. Adding them would cause non-agent and non-blueprint pages to unfurl with incorrect metadata. Per-page `meta()` exports are the correct scope.

**6. Satori requires inline styles.**  
Satori does not support Tailwind classes, CSS modules, or external stylesheets — only a flexbox-compatible subset of inline CSS. The `BlueprintCard` component MUST use inline style objects exclusively. It should not be confused with or share styles from the equivalent UI component, which uses Tailwind. They are maintained separately: one for browser rendering, one for server image generation.
