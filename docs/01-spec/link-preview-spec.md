# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

This spec covers two distinct unfurl behaviors and one intentional non-behavior:

**Case 1 — Share button on agent card.**
The user clicks the three-dot menu on a deployed agent card and selects "Share on LinkedIn" or "Share on X". This opens the platform's native share intent with a pre-filled message containing the agent description and the **blueprint URL**. What unfurls is the **blueprint card** — the same endpoint and result as Case 2. No tokens are minted; the blueprint URL is the thing being shared.

**Case 2 — Blueprint URL pasted anywhere.**
A raw blueprint URL (e.g., `https://astropod.ai/sohumdalal/release-note-helper`) pasted into LinkedIn, X, or Slack unfurls as the **blueprint card** only if the blueprint's visibility is set to **public**. If the blueprint is private, the badge endpoint returns 404 and no OG tags are emitted — the link appears as plain text. Slack is the primary internal testing ground for blueprint unfurling.

**Case 3 — Deployment URL pasted anywhere.**
A raw deployment URL (e.g., `https://astropod.ai/sohumdalal/agents/abg-ieb-2i9`) returns a 404 or not-authorized page and emits no OG metadata. There is no unfurl. Deployments are org-scoped and are not discoverable via raw URL sharing.

Cases 1 and 2 converge on the same blueprint badge endpoint — there is no separate social unfurl path for agents. The agent badge (trading card) is a download artifact only.

---

## Problem Statement

Sharing an Astro link today produces a weak or absent unfurl depending on the page type:

- **Blueprint pages** (`BlueprintDetail.tsx`) already emit `og:title`, `og:description`, `og:image`, and `twitter:card` tags. However, the `og:image` is set to `${assetsBase}/avatars/${account}.jpg` — the account's profile photo. This is generic: every blueprint owned by the same account shows the same image, with no visual information about the blueprint itself.
- **Agent detail pages** emit no OG metadata whatsoever. Pasted agent URLs appear as plain text on all platforms.

The core technical gap is the same in both cases: there is no server-side mechanism to generate a PNG image that represents a specific entity. The `astro-trading-card` package produces SVG strings, and SVG cannot be served as an `og:image` — platforms like LinkedIn and X require a raster image (PNG or JPEG) at that URL.

---

## Access Control Boundary

Not all Astro pages can be unfurled today, and the distinction is not philosophical — it is a product readiness question.

**How unfurling actually works:**

When a user pastes a URL into LinkedIn or X, the platform dispatches an unauthenticated crawler bot to fetch that URL. The bot has no session, no cookie, and no concept of org membership. It reads the `og:image` URL from the HTML response and fetches the PNG separately — also unauthenticated. Platforms then cache the result aggressively.

This means session-aware OG tag injection is not a reliable access control mechanism on its own. Even if the page SSR withholds `og:image` for unauthenticated requests, a user with legitimate access who unfurls the link causes the platform to cache the card. A user without access who pastes the same URL later may receive the cached version.

**Blueprint unfurling is gated on visibility.** Blueprints have a user-controlled visibility setting (public or private). The badge endpoint checks this field before generating a PNG:

- **Public blueprint:** badge renders and OG tags are emitted. The blueprint is intentionally discoverable.
- **Private blueprint:** badge endpoint returns 404 and the page emits no OG tags. The link appears as plain text — no card, no image, no metadata disclosure.

This means visibility is enforced in two places: the badge PNG endpoint (returns 404 if private) and the `BlueprintDetail.tsx` loader (omits OG tags if private). Both checks are necessary — a crawler that somehow has the badge URL directly should also get 404.

**Raw deployment URLs intentionally return 404 or not-authorized with no OG metadata.** Pasting a raw deployment URL into LinkedIn or X produces no unfurl. This is a deliberate product decision: until fine-grained access control (FGAC) exists, there is no safe way to determine whether a given viewer is a member of the org that owns the deployment. Without that check, any unfurl would effectively make deployment existence discoverable to anyone with the URL.

The Share button on the agent card sidesteps this problem entirely by sharing the **blueprint URL** instead of the deployment URL. The blueprint is the public-facing entity; the deployment is the private one. FGAC is not a prerequisite for this flow.

---

## Goals

- **G1:** Public blueprint URLs unfurl with a blueprint-specific landscape card PNG (replacing the current account avatar). Private blueprint URLs emit no OG tags and return 404 from the badge endpoint.
- **G2:** The Share button on the agent card pre-fills a platform share intent with the agent description and the blueprint URL. What unfurls is the blueprint card.
- **G3:** The agent trading card is downloadable as SVG or high-quality PNG from the same Share menu.
- **G4:** Raw deployment URLs return no OG metadata — no unfurl.
- **G5:** All server-rendered PNGs are generated on demand with no browser involved.
- **G6:** PNG endpoints are cacheable at the HTTP layer to avoid repeated re-renders.
- **G7:** The implementation does not require changes to the Go backend.

## Non-Goals

- A separate social unfurl for the agent badge / trading card (blueprint card is the social artifact).
- Deployment page OG tags (raw deployment URLs explicitly have no unfurl).
- Animated or video preview cards.
- Uploading pre-generated PNGs to S3/CDN (can be layered on as a caching optimization later).
- Per-user or per-session personalization of the card image.
- Changing the visual design of the trading card itself.

---

## Design

### Overview

There is one badge route that matters for social sharing: the blueprint badge. The agent badge route exists for PNG download only. Both are handled by the Bun server before the React Router handler runs.

```
[Social platform crawler]
  GET /blueprints/:account/:name    (blueprint page)
  -> reads og:image from HTML
  -> GET /badge/blueprint/:account/:name.png

[Bun SSR server]
  GET /badge/blueprint/:account/:name.png
    1. Fetch blueprint from API
    2. Check visibility -- 404 if private
    3. blueprintToCardProps()
    4. satori(BlueprintCard)   -- JSX -> SVG
    5. Resvg().render()        -- SVG -> PNG
    6. Return PNG (cached)

  GET /badge/agent/:account/:name.png   (download only)
    1. Fetch agent from API
    2. agentToCardData()
    3. generateCard()          -- astro-trading-card
    4. Resvg().render()        -- SVG -> PNG
    5. Return PNG
```

---

### 1. Blueprint Badge Endpoint

**Route:** `GET /badge/blueprint/:account/:name.png`

Renders a landscape card that mirrors the blueprint listing UI. This is the only social-unfurl image endpoint. Portrait format is intentionally not used for social sharing — see Design Decision #3.

This endpoint MUST check the blueprint's visibility before rendering. If the blueprint is private, it returns `404 Not Found` with no body. This prevents the badge from being accessible even if someone constructs the URL directly.

#### Response

| Scenario | Status | Body |
|----------|--------|------|
| Success (public blueprint) | `200 OK` | PNG binary, `Content-Type: image/png` |
| Private blueprint | `404 Not Found` | Empty body |
| Blueprint not found | `404 Not Found` | Empty body |
| Upstream API error | `502 Bad Gateway` | Empty body |

The `BlueprintDetail.tsx` loader MUST also check visibility and omit all OG tags when the blueprint is private — so neither the page HTML nor the badge URL leaks any information.

#### Visual Design

The blueprint card matches the blueprint listing UI card design:

```
+----------------------------------------------------------+
|  . . . . . . . . . . . . . . . . . . .  (dot grid bg)   |
|                                                          |
|  [avatar]  release-note-helper                           |
|            An agent that helps you craft release notes   |
|            from Jira issues and GitHub PRs               |
|                                                          |
|  -------------------------------------------------------- |
|  0 deploys                            (o) sohumdalal     |
+----------------------------------------------------------+
```

| Property | Value |
|----------|-------|
| Dimensions | 1200x630px |
| Background | Light gray (`#f8f9fa`) with dot grid overlay |
| Icon | Rounded square, 80x80px, agent avatar or initials fallback |
| Title | Bold, ~28px, dark gray (`#111827`) |
| Description | Regular, ~18px, medium gray (`#6b7280`), max 2 lines |
| Divider | 1px line, light gray (`#e5e7eb`) |
| Footer left | Deploy count, ~16px, medium gray |
| Footer right | Owner avatar (24px circle) + handle, ~16px |
| Corner radius | 12px |
| Border | 1px, `#e5e7eb` |

#### Rendering Approach: Satori

Blueprint cards are rendered with **Satori** (`satori` npm package), which converts a JSX component tree to an SVG string using flexbox layout — the same technology behind `@vercel/og`. The SVG is then passed to Resvg for PNG conversion.

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
  if (!blueprint.public) return new Response(null, { status: 404 });

  const props = blueprintToCardProps(blueprint);
  const element = BlueprintCard(props);
  const svg = await satori(element, {
    width: 1200,
    height: 630,
    fonts: [
      { name: "Inter", data: font, weight: 400 },
      { name: "Inter", data: fontBold, weight: 700 },
    ],
  });

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

#### Blueprint Data -> CardProps Mapping

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

### 2. Agent Badge Endpoint (Download Only)

**Route:** `GET /badge/agent/:account/:name.png`

Renders the agent's `astro-trading-card` as a PNG. Used only for the "Download PNG" option in the Share menu — this endpoint is NOT referenced in any OG tag and is NOT the target of social crawlers.

#### Response

| Scenario | Status | Body |
|----------|--------|------|
| Success | `200 OK` | PNG binary, `Content-Type: image/png` |
| Agent not found | `404 Not Found` | Empty body |
| Upstream API error | `502 Bad Gateway` | Empty body |

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

#### Agent Data -> CardData Mapping

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

### 3. Open Graph Meta Tags

React Router v7 supports per-route `meta()` exports that run during SSR.

The `host` MUST be derived from `new URL(request.url).host` in the loader and returned as part of loader data — never hardcoded — to work correctly across local, staging, and production environments.

#### Blueprint Detail Page

`BlueprintDetail.tsx` already exports a `meta()` function with `og:title`, `og:description`, `og:url`, `og:image`, and `twitter:card`. The only change required is replacing the `ogImage` value passed from the loader — currently `${assetsBase}/avatars/${account}.jpg` — with the new badge endpoint URL. All other tags remain unchanged.

OG tags MUST be omitted entirely when the blueprint is private.

| Property | Current value | New value |
|----------|--------------|-----------|
| `og:image` | `${assetsBase}/avatars/${account}.jpg` | `https://{host}/badge/blueprint/{account}/{name}.png` |
| `og:image:width` | _(not set)_ | `1200` |
| `og:image:height` | _(not set)_ | `630` |
| `twitter:image` | Same as `og:image` | Same as new `og:image` |
| All other tags | Unchanged | Unchanged |

#### Agent Detail Page

No OG tags are added to the agent detail page. Deployment URLs are intentionally dark (Case 3). The agent detail page does not participate in social unfurling.

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

## In-App Share Flow (Agent Card)

The deployed agent card has a three-dot menu. A "Share" option in that menu exposes two distinct actions: download and social share.

### Download

| Option | Behavior |
|--------|----------|
| Download SVG | Client-side. Calls `downloadSvg()` from `astro-trading-card/browser`. No server request. |
| Download PNG | Server-side. Fetches `/badge/agent/:account/:name.png`, which uses Resvg for a high-quality render. Triggered as a file download in the browser. |

The server-side PNG download is preferred over the existing canvas-based `downloadPng()` because Resvg produces sharper output (especially at 2x scale) and is consistent across browsers.

### Share to Social Platform

When the user selects a social platform, the client opens the platform's native share intent URL with:

1. The **blueprint URL** as the shared link (e.g., `https://astropod.ai/{account}/{blueprintName}`)
2. A **pre-filled message** containing the agent display name and blueprint link

The blueprint URL is what the platform crawler fetches and unfurls — it hits the blueprint page, reads `og:image`, and serves the blueprint badge PNG. This is identical to what happens in Case 2. No tokens are minted.

**Pre-filled message templates:**

| Platform | Message |
|----------|---------|
| LinkedIn | "Just deployed {displayName} on Astro using the {blueprint} blueprint. Check it out: {blueprintUrl}" |
| X (Twitter) | "Just deployed {displayName} on Astro using {blueprint} {blueprintUrl}" |

**Share intent URLs:**

```
LinkedIn: https://www.linkedin.com/sharing/share-offsite/?url={encodedBlueprintUrl}
X:        https://x.com/intent/post?text={encodedMessage}&url={encodedBlueprintUrl}
```

All values MUST be URL-encoded. The blueprint URL, display name, and blueprint name are available from the deployment page's existing loader data.

---

## Files to Modify or Create

| File | Change |
|------|--------|
| `apps/astro-client/server.ts` | Add `/badge/agent/*` and `/badge/blueprint/*` routes before React Router handler |
| `apps/astro-client/src/badge-agent.ts` | **New.** `agentToCardData()` and `generateAgentBadgePng()` (download only) |
| `apps/astro-client/src/badge-blueprint.tsx` | **New.** `BlueprintCard` JSX component (Satori-compatible, inline styles only) and `generateBlueprintBadgePng()` |
| `apps/astro-client/src/badge-cache.ts` | **New.** Shared LRU cache instance |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add three-dot Share menu: Download SVG, Download PNG, share intent links (blueprint URL) |
| `apps/astro-client/src/pages/blueprints/BlueprintDetail.tsx` | Replace `ogImage` in loader with badge endpoint URL; add `og:image:width` and `og:image:height`; omit OG tags if private |
| `apps/astro-client/package.json` | Add `@resvg/resvg-js`, `satori`, `lru-cache` |

---

## Implementation Order

**Phase 1 — Blueprint badge + unfurl (no visible user change until Phase 2):**
1. Install `@resvg/resvg-js`, `satori`, `lru-cache`
2. Create `src/badge-blueprint.tsx` and `src/badge-cache.ts`
3. Add `/badge/blueprint/:account/:name.png` handler in `server.ts`
4. Verify: `curl http://localhost:3000/badge/blueprint/postman/release-note-helper.png > blueprint.png`
5. In `BlueprintDetail.tsx` loader, replace `ogImage` with badge endpoint URL; add width/height tags; omit all OG tags if private
6. Verify unfurl with LinkedIn Post Inspector and Twitter Card Validator against staging; paste blueprint URL into a Slack channel as a final check

**Phase 2 — Agent card Share menu + download:**
1. Create `src/badge-agent.ts`
2. Add `/badge/agent/:account/:name.png` handler in `server.ts`
3. Add three-dot Share menu to `DeployedAgentDetail.tsx`
4. Download SVG: client-side via `downloadSvg()` (already available)
5. Download PNG: fetch `/badge/agent/*` endpoint, trigger browser download
6. Share to LinkedIn / X: construct intent URL with blueprint URL + pre-filled message
7. Verify: share from agent card on each platform; confirm blueprint card unfurls (not agent badge)

Phase 1 is the highest-value work and ships independently. Phase 2 ships as a single unit since the agent endpoint and the share menu ship together.

---

## Key Design Decisions

**1. PNG generation location: Bun server (current recommendation) vs. Go backend (investigate).**  
The `astro-trading-card` package and Satori are TypeScript libraries, which makes the Bun server the natural home for PNG generation. However, the Go backend already contains a pure-Go SVG rasterization pipeline (`oksvg` + `rasterx`, in `internal/identitygen/raster.go`) used for avatar generation. This is worth investigating before committing to the Bun server approach.

If the Go rasterizer can handle the SVG output of both `astro-trading-card` and Satori, the badge endpoints could be served as proper Go API routes — no new Bun dependencies, unified caching, and badges accessible from the same origin as the rest of the API.

The risk is SVG compatibility. `oksvg`/`rasterx` supports a limited SVG subset (paths, basic shapes, gradients) and may not handle the trading card's use of `clipPath`, `mask`, embedded images, and custom font rendering. Satori's output is simpler and more likely to be compatible. **Required investigation before Phase 1:** render a sample trading card SVG and a sample Satori blueprint SVG through the existing Go rasterizer and compare output quality. If both render correctly, move badge generation to Go. If only the blueprint renders correctly, use Go for blueprints and `@resvg/resvg-js` in Bun for the agent badge.

**2. Different rendering stacks for agents vs. blueprints.**  
Agent cards reuse the existing `astro-trading-card` package (which already produces polished SVG output) plus Resvg. Blueprint cards use Satori because the target visual is the existing UI card component — JSX with inline styles is the most maintainable way to reproduce and keep that in sync. A single approach for both would require either forcing the UI-style card into a hand-written SVG template (brittle) or rebuilding the trading card in Satori (unnecessary work).

**3. The agent trading card is a download artifact, not a social broadcast.**  
The agent trading card (portrait, dark, holographic) is a personal trophy — something the user downloads and keeps. It is not used as an OG image. The Share button on the agent card shares the **blueprint URL** instead, which unfurls with the blueprint landscape card. This is correct for two reasons: (a) the trading card is portrait-format and letterboxes poorly on every platform's `summary_large_image` layout, and (b) the blueprint — not the deployment — is the public-facing entity that makes sense for someone seeing it cold. The deployment is org-scoped; the blueprint is the thing others can actually act on (clone, deploy, explore).

**4. On-demand generation, not pre-generation.**  
Pre-generating PNGs at creation time would require the backend to know about the Bun server (or a separate image service), adding coupling. On-demand generation is simpler, and the LRU + HTTP cache provides sufficient performance for crawlers, which are low-frequency compared to regular user traffic.

**5. No OG tags in `root.tsx`.**  
Global fallback OG tags are intentionally omitted from `root.tsx`. Adding them would cause non-agent and non-blueprint pages to unfurl with incorrect metadata. Per-page `meta()` exports are the correct scope.

**6. Satori requires inline styles.**  
Satori does not support Tailwind classes, CSS modules, or external stylesheets — only a flexbox-compatible subset of inline CSS. The `BlueprintCard` component MUST use inline style objects exclusively. It should not be confused with or share styles from the equivalent UI component, which uses Tailwind. They are maintained separately: one for browser rendering, one for server image generation.
