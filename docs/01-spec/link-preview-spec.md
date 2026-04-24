# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

When a user pastes an Astro URL into Slack, LinkedIn, X (Twitter), or any Open Graph-aware platform, the platform crawls that URL and renders an unfurl card — a rich preview with a title, description, and image. Blueprint pages already emit Open Graph metadata, but the preview image is the **account's profile avatar** — a generic photo with no connection to the blueprint itself.

This spec does two distinct things:

**1. Blueprint link unfurling.** When a blueprint URL is shared anywhere, it unfurls with a card that mirrors the blueprint listing UI — clean, light, landscape — showing the blueprint's name, description, deploy count, and owner. This is the primary social sharing surface.

**2. Agent card download.** The deployed agent trading card (dark, holographic, portrait) is a downloadable artifact — an SVG or high-quality PNG the user can save. It is not used as an OG image. When a user shares from the agent card via the in-app Share menu, they share the *blueprint* URL (with a pre-filled message), not the deployment URL. What unfurls is the blueprint card, not the trading card.

This separation keeps the social surface clean: blueprints are the public-facing entity, deployments are org-scoped. The trading card is a trophy, not a broadcast.

---

## Problem Statement

Sharing an Astro link today produces a weak or absent unfurl depending on the page type:

- **Blueprint pages** (`BlueprintDetail.tsx`) already emit `og:title`, `og:description`, `og:image`, and `twitter:card` tags. However, the `og:image` is set to `${assetsBase}/avatars/${account}.jpg` — the account's profile photo. This is generic: every blueprint owned by the same account shows the same image, with no visual information about the blueprint itself.
- **Agent detail pages** emit no OG metadata whatsoever. Pasted agent URLs appear as plain text on all platforms.

The core technical gap is the same in both cases: there is no server-side mechanism to generate a PNG image that represents a specific entity. The `astro-trading-card` package produces SVG strings, and SVG cannot be served as an `og:image` — platforms like LinkedIn, X, and Slack require a raster image (PNG or JPEG) at that URL.

---

## Access Control Boundary

Not all Astro pages can be unfurled today, and the distinction is not philosophical — it is a product readiness question.

**How unfurling actually works:**

When a user pastes a URL into Slack or LinkedIn, the platform dispatches an unauthenticated crawler bot to fetch that URL. The bot has no session, no cookie, and no concept of org membership. It reads the `og:image` URL from the HTML response and fetches the PNG separately — also unauthenticated. Platforms then cache the result aggressively.

This means session-aware OG tag injection is not a reliable access control mechanism on its own. Even if the page SSR withholds `og:image` for unauthenticated requests, a user with legitimate access who unfurls the link causes the platform to cache the card. A user without access who pastes the same URL later may receive the cached version.

**Blueprints are safe to unfurl now** because:
- Blueprints are designed to be shared — the product intent is discovery.
- The badge shows only information the URL already implies: name, description, deploy count, owner handle. Nothing in the card is a data disclosure beyond what knowing the blueprint slug already reveals.
- For private blueprints, access control is enforced on the page itself. The card is a cover image, not a data leak.

**Deployment pages cannot be unfurled yet — this is a prerequisite gap, not a permanent decision.**

The intended end state for deployment unfurling is org-scoped sharing: if you share a deployment URL with a colleague in the same org, they can open it; anyone outside the org is denied — the same model as a private GitHub repository. This is the right product behavior.

The blocker is that **fine-grained access control (FGAC) does not exist yet**. Without FGAC, there is no mechanism to:
1. Determine whether a given viewer is a member of the org that owns the deployment.
2. Gate the badge PNG endpoint on that membership.
3. Prevent a card generated for an in-org user from being served (via platform cache) to an out-of-org user.

Once FGAC is in place, the path forward for deployment unfurling is an explicit **"Share" flow** rather than automatic OG tags on the deployment page:

1. An in-org user clicks a "Copy share link" button on the deployment page.
2. FGAC confirms the user has share permission for that deployment.
3. The server generates a time-limited, signed token and returns a share URL: `/share/d/:token`.
4. That URL renders a minimal deployment card (agent name, org, status — nothing sensitive) and emits the OG tags.
5. Anyone with the link can see the card; the token expiry bounds the exposure window. The underlying deployment page still requires org membership.

This model keeps the badge stateless (the token carries the authorization) while letting FGAC control who can generate share links in the first place. It is intentionally out of scope for this spec.

---

## Goals

- **G1:** Blueprint page URLs replace the current account-avatar `og:image` with a UI-style landscape card PNG specific to the blueprint.
- **G2:** The deployed agent card (trading card) is downloadable as SVG or PNG from a three-dot Share menu.
- **G3:** Sharing to LinkedIn, X, or Slack from the agent card pre-fills a message with the blueprint URL — what unfurls is the blueprint card, not the trading card.
- **G4:** All server-rendered PNGs are generated on demand with no browser involved.
- **G5:** PNG endpoints are cacheable at the HTTP layer to avoid repeated re-renders.
- **G6:** The implementation does not require changes to the Go backend.

## Non-Goals

- Deployment page OG tags or unfurling (see Access Control section).
- Agent trading card as an OG image — the card is a download artifact only.
- Animated or video preview cards.
- Uploading pre-generated PNGs to S3/CDN (can be layered on as a caching optimization later).
- Per-user or per-session personalization of the card image.
- Changing the visual design of the trading card itself.

---

## Design

### Overview

There are two independent badge routes, one per page type. Both are handled by the Bun server before the React Router handler runs and neither is proxied to the Go backend.

```
[Social platform crawler]
  GET /:account/:name               (agent page)
  GET /blueprints/:account/:name    (blueprint page)

[Bun SSR server - React Router SSR]
  meta() export returns og:image pointing at badge route

  GET /badge/agent/:account/:name.png
    1. Fetch agent from API
    2. agentToCardData()
    3. generateCard()         -- astro-trading-card
    4. Resvg().render()       -- @resvg/resvg-js
    5. Return PNG

  GET /badge/blueprint/:account/:name.png
    1. Fetch blueprint from API
    2. blueprintToCardProps()
    3. satori(BlueprintCard)  -- satori (JSX -> SVG)
    4. Resvg().render()       -- @resvg/resvg-js
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

  const element = BlueprintCard(props); // JSX: <BlueprintCard {...props} />
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

## In-App Share Flow (Agent Card)

The deployed agent card has a three-dot menu. A "Share" option in that menu exposes two distinct actions:

### Download

| Option | Behavior |
|--------|----------|
| Download SVG | Client-side. Calls `downloadSvg()` from `astro-trading-card/browser`. No server request. |
| Download PNG | Server-side. Fetches `/badge/agent/:account/:name.png`, which uses Resvg for a high-quality render. Triggered as a file download in the browser. |

The server-side PNG download is preferred over the existing canvas-based `downloadPng()` because Resvg produces sharper output (especially at 2x scale) and is consistent across browsers.

### Share to Social Platform

When the user selects a social platform (LinkedIn, X, or Slack), the app opens that platform's share intent URL with a pre-filled message. The shared URL is the **blueprint URL**, not the deployment URL.

**Why the blueprint URL, not the deployment URL:**
- Deployments are org-scoped. Sharing a deployment URL with someone outside the org is meaningless — they cannot access it.
- Blueprints are the public-facing entity. The blueprint represents what the agent *is*; the deployment is an instance of it.
- The blueprint card unfurls cleanly, giving the recipient context without requiring org access.

**Pre-filled message templates:**

| Platform | Message |
|----------|---------|
| LinkedIn | "Just deployed {displayName} on Astro using the {blueprint} blueprint. {blueprintUrl}" |
| X (Twitter) | "Just deployed {displayName} on Astro using {blueprint} {blueprintUrl}" |
| Slack | Deep link to Slack compose with the same message body |

The `{blueprintUrl}` is the canonical blueprint page URL (e.g., `https://astropod.ai/{account}/{blueprintName}`). When the recipient's platform crawls this URL, it fetches the blueprint badge PNG and renders the blueprint card — not the trading card.

**Share intent URLs:**

```
LinkedIn: https://www.linkedin.com/sharing/share-offsite/?url={encodedBlueprintUrl}
X:        https://x.com/intent/post?text={encodedMessage}&url={encodedBlueprintUrl}
Slack:    https://slack.com/intl/en-us/share?url={encodedBlueprintUrl}
```

The message and URL MUST be URL-encoded before being appended to the intent URL. The blueprint URL and display name are available from the deployment page's existing loader data (each deployment knows its source blueprint).

---

## Files to Modify or Create

| File | Change |
|------|--------|
| `apps/astro-client/server.ts` | Add `/badge/agent/*` and `/badge/blueprint/*` routes before React Router handler |
| `apps/astro-client/src/badge-agent.ts` | **New.** `agentToCardData()` and `generateAgentBadgePng()` — serves PNG downloads, not OG images |
| `apps/astro-client/src/badge-blueprint.tsx` | **New.** `BlueprintCard` JSX component (Satori-compatible, inline styles only) and `generateBlueprintBadgePng()` |
| `apps/astro-client/src/badge-cache.ts` | **New.** Shared LRU cache instance |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add Share menu (three-dot) with Download SVG, Download PNG, and social share options; no OG meta tags |
| `apps/astro-client/src/pages/blueprints/BlueprintDetail.tsx` | Replace `ogImage` in loader with badge endpoint URL; add `og:image:width` and `og:image:height` |
| `apps/astro-client/package.json` | Add `@resvg/resvg-js`, `satori`, `lru-cache` |

---

## Implementation Order

**Phase 1 — Blueprint badge PNG endpoint (no visible user change):**
1. Install `@resvg/resvg-js`, `satori`, and `lru-cache`
2. Create `src/badge-blueprint.tsx`, `src/badge-cache.ts`
3. Add `/badge/blueprint/:account/:name.png` handler in `server.ts`
4. Verify: `curl http://localhost:3000/badge/blueprint/postman/release-note-helper.png > blueprint.png`

**Phase 2 — Blueprint OG meta tags (enables unfurling):**
1. In `BlueprintDetail.tsx` loader, replace `ogImage` derivation with the badge endpoint URL
2. Add `og:image:width` (1200) and `og:image:height` (630) to the existing `meta()` export
3. Verify with LinkedIn Post Inspector and Twitter Card Validator against a staging URL
4. Verify Slack unfurl by pasting a blueprint URL into a test channel

**Phase 3 — Agent card download (server-side PNG):**
1. Create `src/badge-agent.ts`
2. Add `/badge/agent/:account/:name.png` handler in `server.ts` (download use only, not OG)
3. Wire "Download PNG" in the Share menu to fetch this endpoint and trigger a browser download

**Phase 4 — Agent card Share menu (social share intents):**
1. Add three-dot Share menu to `DeployedAgentDetail.tsx`
2. Implement Download SVG (client-side, `downloadSvg()` already available)
3. Implement Download PNG (fetch `/badge/agent/*` endpoint, trigger download)
4. Implement LinkedIn / X / Slack share intent links with pre-filled blueprint URL and message
5. Verify intent URLs open correctly with expected pre-filled text on each platform

Phases 1 and 2 are the primary unfurling work and ship as a unit. Phases 3 and 4 are the agent card share UX and can ship independently after.

---

## Key Design Decisions

**1. PNG generation in the Bun server, not the Go backend.**  
The `astro-trading-card` package and Satori are both TypeScript libraries. Calling them from Go would require spawning a subprocess or duplicating layout logic — both are poor options. The Bun server already runs TypeScript and is the natural owner of this work. The Go backend is not modified.

**2. Different rendering stacks for agents vs. blueprints.**  
Agent cards reuse the existing `astro-trading-card` package (which already produces polished SVG output) plus Resvg. Blueprint cards use Satori because the target visual is the existing UI card component — JSX with inline styles is the most maintainable way to reproduce and keep that in sync. A single approach for both would require either forcing the UI-style card into a hand-written SVG template (brittle) or rebuilding the trading card in Satori (unnecessary work).

**3. On-demand generation, not pre-generation.**  
Pre-generating PNGs at creation time would require the backend to know about the Bun server (or a separate image service), adding coupling. On-demand generation is simpler, and the LRU + HTTP cache provides sufficient performance for crawlers, which are low-frequency compared to regular user traffic.

**4. The trading card is a download artifact, not a social broadcast.**  
The agent trading card (portrait, dark, holographic) is a personal trophy — something the user downloads and keeps. It is not suitable as an OG image for two reasons: (a) it is 350x560px portrait, which letterboxes poorly on every major platform's `summary_large_image` format, and (b) it represents a deployment, which is org-scoped and should not be the thing that reaches people outside the org. Social sharing from the agent card routes to the blueprint instead, which is landscape, public-facing, and carries the right context for someone seeing it cold.

**5. No OG tags in `root.tsx`.**  
Global fallback OG tags are intentionally omitted from `root.tsx`. Adding them would cause non-agent and non-blueprint pages to unfurl with incorrect metadata. Per-page `meta()` exports are the correct scope.

**6. Satori requires inline styles.**  
Satori does not support Tailwind classes, CSS modules, or external stylesheets — only a flexbox-compatible subset of inline CSS. The `BlueprintCard` component MUST use inline style objects exclusively. It should not be confused with or share styles from the equivalent UI component, which uses Tailwind. They are maintained separately: one for browser rendering, one for server image generation.
