# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

This spec covers two distinct unfurl behaviors and one intentional non-behavior:

**Cases 1 and 2 converge on the same path.**
Whether a user clicks Share anywhere in the app (Case 1) or pastes a blueprint URL directly into LinkedIn, X, or Slack (Case 2), the outcome is identical: the flow hits a single visibility gate ("is this blueprint public?"), and if yes, the blueprint card PNG unfurls. If no, nothing unfurls. There is no badge endpoint for the agent card — SVG and PNG downloads for the trading card are already handled by the existing `downloadSvg()` and `downloadPng()` functions in the codebase.

**Case 1 — Share button (anywhere in the app).**
Clicking Share routes to the `blueprint public?` gate. If the blueprint is public, the platform crawls the blueprint URL and renders the blueprint card. A pre-filled message includes the agent description and blueprint link. Currently only public blueprints can be shared this way; FGAC will later enable private-scope sharing for org members.

**Case 2 — Blueprint URL pasted anywhere.**
A raw blueprint URL (e.g., `https://astropod.ai/sohumdalal/release-note-helper`) pasted into LinkedIn, X, or Slack hits the same `blueprint public?` gate. Public = blueprint card unfurls. Private = 404, no card, plain text link. Slack is the primary internal testing ground.

**Case 3 — Deployment URL pasted anywhere.**
A raw deployment URL returns 404 or not-authorized with no OG metadata. No unfurl. Until FGAC exists, there is no way to gate deployment unfurling on org membership, so deployment URLs are intentionally dark.

---

## Problem Statement

Sharing an Astro link today produces a weak or absent unfurl depending on the page type:

- **Blueprint pages** (`BlueprintDetail.tsx`) already emit `og:title`, `og:description`, `og:image`, and `twitter:card` tags. However, the `og:image` is set to `${assetsBase}/avatars/${account}.jpg` — the account's profile photo. This is generic: every blueprint owned by the same account shows the same image, with no visual information about the blueprint itself.
- **Agent detail pages** emit no OG metadata whatsoever. Pasted agent URLs appear as plain text on all platforms.

The core technical gap is the same in both cases: there is no server-side mechanism to generate a PNG image that represents a specific entity. The `astro-trading-card` package produces SVG strings, and SVG cannot be served as an `og:image` — platforms like LinkedIn and X require a raster image (PNG or JPEG) at that URL.

---

## Access Control Boundary

Not all Astro pages can be unfurled today, and the distinction is a product readiness question, not a philosophical one.

**How unfurling actually works:**

When a URL is pasted into LinkedIn or X, the platform dispatches an unauthenticated crawler bot. The bot has no session, no cookie, and no concept of org membership. It reads the `og:image` URL from the HTML response and fetches the PNG separately — also unauthenticated. Platforms cache the result aggressively.

This means session-aware OG tag injection is not a reliable access control mechanism. Even if the page SSR withholds `og:image` for unauthenticated requests, a user with legitimate access who unfurls the link causes the platform to cache the card. A user without access who pastes the same URL later may receive the cached version.

**Blueprint unfurling is gated on visibility.** Blueprints have a user-controlled visibility setting (public or private). The same gate applies to both the Share button flow (Case 1) and the raw URL paste flow (Case 2):

- **Public blueprint:** badge renders and OG tags are emitted. The blueprint is intentionally discoverable.
- **Private blueprint:** badge endpoint returns 404 and the page emits no OG tags. The link appears as plain text — no card, no image, no metadata disclosure.

Visibility is enforced in two places: the badge PNG endpoint (returns 404 if private) and the `BlueprintDetail.tsx` loader (omits OG tags if private). Both checks are necessary — a crawler that somehow has the badge URL directly should also get 404.

**Once FGAC exists**, the Share button in Case 1 will be able to gate sharing on org membership, unlocking private blueprint sharing for authorized members. The underlying badge infrastructure does not change; FGAC adds the pre-condition.

**Raw deployment URLs intentionally return 404 or not-authorized with no OG metadata.** Until FGAC exists, there is no safe way to determine whether a given viewer is a member of the org that owns the deployment. Deployment URLs are intentionally dark. FGAC is the prerequisite for any deployment unfurl story.

---

## Goals

- **G1:** Public blueprint URLs unfurl with a blueprint-specific landscape card PNG (replacing the current account avatar). Private blueprint URLs emit no OG tags and return 404 from the badge endpoint.
- **G2:** The Share button (anywhere in the app) routes through the same blueprint visibility gate and produces the same unfurl as pasting the blueprint URL directly.
- **G3:** The Share button pre-fills a platform message with the agent description and the blueprint URL.
- **G4:** The agent trading card is downloadable as SVG and PNG from the Share menu using the existing `downloadSvg()` and `downloadPng()` functions — no new server endpoint required.
- **G5:** Raw deployment URLs return no OG metadata — no unfurl.
- **G6:** All server-rendered PNGs are generated on demand with no browser involved.
- **G7:** PNG endpoints are cacheable at the HTTP layer to avoid repeated re-renders.
- **G8:** The implementation does not require changes to the Go backend.

## Non-Goals

- A separate social unfurl for the agent badge / trading card (blueprint card is the only social artifact).
- Deployment page OG tags (raw deployment URLs explicitly have no unfurl until FGAC).
- Private blueprint sharing (blocked until FGAC).
- Animated or video preview cards.
- Uploading pre-generated PNGs to S3/CDN (can be layered on as a caching optimization later).
- Per-user or per-session personalization of the card image.
- Changing the visual design of the trading card itself.

---

## Design

### Overview

There is one badge endpoint: the blueprint badge. Both Cases 1 and 2 flow through the same blueprint visibility gate and the same badge endpoint. Agent card downloads use existing client-side functions — no server endpoint.

```
Case 1: User clicks Share (anywhere in app)
Case 2: User pastes blueprint URL into LinkedIn / X / Slack
         |
         v
    blueprint public?
         |         \
        YES         NO --> 404, no unfurl
         |
         v
GET /badge/blueprint/:account/:name.png
  1. Fetch blueprint from API
  2. Check visibility -- 404 if private
  3. blueprintToCardProps()
  4. satori(BlueprintCard)   -- JSX -> SVG
  5. Resvg().render()        -- SVG -> PNG
  6. Return PNG (LRU cached)
         |
         v
Blueprint Card PNG (1200x630, landscape)
         |
         v
LinkedIn  ·  X  ·  Slack


Case 3: User pastes deployment URL anywhere
         |
         v
404 / Not Authorized
No OG tags emitted
No unfurl
(Until FGAC: no org membership check possible at OG layer)


Agent card downloads (already implemented -- no new endpoint):
  downloadSvg()  -- client-side, astro-trading-card/browser
  downloadPng()  -- client-side, existing canvas-based export
```

---

### 1. Blueprint Badge Endpoint

**Route:** `GET /badge/blueprint/:account/:name.png`

The only social-unfurl image endpoint. Both the Share button (Case 1) and the raw blueprint URL (Case 2) cause platform crawlers to hit this endpoint when the blueprint is public.

#### Response

| Scenario | Status | Body |
|----------|--------|------|
| Success (public blueprint) | `200 OK` | PNG binary, `Content-Type: image/png` |
| Private blueprint | `404 Not Found` | Empty body |
| Blueprint not found | `404 Not Found` | Empty body |
| Upstream API error | `502 Bad Gateway` | Empty body |

The `BlueprintDetail.tsx` loader MUST also check visibility and omit all OG tags when the blueprint is private — so neither the page HTML nor the badge URL leaks any information.

#### Visual Design

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

Blueprint cards are rendered with **Satori** (`satori` npm package), which converts a JSX component tree to an SVG string using flexbox layout. The SVG is then passed to Resvg for PNG conversion.

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

The `BlueprintCard` JSX component lives in `src/badge-blueprint.tsx`. It MUST NOT import any browser-only APIs, Tailwind classes, or CSS modules — Satori requires inline styles only.

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

Fonts SHOULD be loaded once at server startup (not per-request) and stored in module scope. Inter (Regular 400, Bold 700) is the recommended typeface to match the UI.

---

### 2. Open Graph Meta Tags

React Router v7 supports per-route `meta()` exports that run during SSR. The `host` MUST be derived from `new URL(request.url).host` in the loader — never hardcoded — to work correctly across local, staging, and production environments.

#### Blueprint Detail Page

`BlueprintDetail.tsx` already exports a `meta()` function. The only required change is replacing the `ogImage` value in the loader with the badge endpoint URL. OG tags MUST be omitted entirely when the blueprint is private.

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

### 3. In-Memory Cache

The blueprint badge endpoint SHOULD cache rendered PNGs in memory.

| Property | Value |
|----------|-------|
| Cache key | `blueprint:${account}/${name}` |
| Cached value | PNG `Uint8Array` |
| Max entries | 500 |
| TTL | 1 hour (3600s) |
| Eviction | LRU |

Use `lru-cache` npm package. The HTTP `Cache-Control` header handles CDN-level caching independently.

---

### 4. Dependency Changes

| Package | Location | Change |
|---------|----------|--------|
| `@resvg/resvg-js` | `apps/astro-client` | Add as dependency |
| `satori` | `apps/astro-client` | Add as dependency |
| `lru-cache` | `apps/astro-client` | Add as dependency |

---

## In-App Share Flow

The Share button can appear anywhere in the app — on the deployed agent card or elsewhere. It exposes two categories of action: **download** and **social share**.

### Download

Agent card downloads are already implemented — no new server endpoint required.

| Option | Behavior |
|--------|----------|
| Download SVG | Client-side. Calls `downloadSvg()` from `astro-trading-card/browser`. No server request. |
| Download PNG | Client-side. Calls the existing `downloadPng()` function. No server request. |

### Share to Social Platform

Clicking "Share on LinkedIn" or "Share on X" opens the platform's native share intent with:

1. The **blueprint URL** as the shared link (e.g., `https://astropod.ai/{account}/{blueprintName}`)
2. A **pre-filled message** containing the agent display name and blueprint URL

The platform crawler fetches the blueprint URL, reads `og:image`, and serves the blueprint badge PNG. This is the same path as Case 2 — the visibility gate applies equally.

**Currently:** only public blueprints unfurl. **Once FGAC exists:** private blueprints can be shared to org members.

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

All values MUST be URL-encoded.

---

## Files to Modify or Create

| File | Change |
|------|--------|
| `apps/astro-client/server.ts` | Add `/badge/blueprint/*` route before React Router handler |
| `apps/astro-client/src/badge-blueprint.tsx` | **New.** `BlueprintCard` JSX (Satori, inline styles only) and `generateBlueprintBadgePng()` |
| `apps/astro-client/src/badge-cache.ts` | **New.** LRU cache instance for blueprint badges |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add Share menu: Download SVG (existing), Download PNG (existing), share intent links (blueprint URL) |
| `apps/astro-client/src/pages/blueprints/BlueprintDetail.tsx` | Replace `ogImage` in loader with badge endpoint URL; add width/height; omit OG tags if private |
| `apps/astro-client/package.json` | Add `@resvg/resvg-js`, `satori`, `lru-cache` |

---

## Implementation Order

**Phase 1 — Blueprint badge + unfurl:**
1. Install `@resvg/resvg-js`, `satori`, `lru-cache`
2. Create `src/badge-blueprint.tsx` and `src/badge-cache.ts`
3. Add `/badge/blueprint/:account/:name.png` handler in `server.ts`
4. Verify: `curl http://localhost:3000/badge/blueprint/postman/release-note-helper.png > blueprint.png`
5. In `BlueprintDetail.tsx` loader, replace `ogImage` with badge endpoint URL; add width/height; omit all OG tags if private
6. Verify unfurl with LinkedIn Post Inspector and Twitter Card Validator against staging; paste blueprint URL into Slack as a final check

**Phase 2 — Agent card Share menu:**
1. Add Share menu to `DeployedAgentDetail.tsx` (and anywhere else it's needed)
2. Download SVG: wire to existing `downloadSvg()`
3. Download PNG: wire to existing `downloadPng()`
4. Share to LinkedIn / X: construct intent URL with blueprint URL + pre-filled message
5. Verify: share from agent card on each platform; confirm blueprint card unfurls

Phase 1 ships independently. Phase 2 ships as a single unit.

---

## Key Design Decisions

**1. PNG generation location: Bun server (current recommendation) vs. Go backend (investigate).**  
Satori is a TypeScript library, making the Bun server the natural home for blueprint PNG generation. However, the Go backend already contains a pure-Go SVG rasterization pipeline (`oksvg` + `rasterx`, in `internal/identitygen/raster.go`) used for avatar generation. This is worth investigating before committing to the Bun server approach.

If the Go rasterizer can handle Satori's SVG output, the badge endpoint could be served as a proper Go API route — no new Bun dependencies, unified caching, same origin as the API. **Required investigation before Phase 1:** render a sample Satori blueprint SVG through the Go rasterizer and compare output quality. If it renders correctly, move badge generation to Go. Otherwise, proceed with Bun + `@resvg/resvg-js`.

**2. Blueprint card visual design requires a dedicated creative pass.**  
The `BlueprintCard` Satori component needs its own visual design — something appealing, specific to blueprints, and distinct from the generic account avatar currently used. The ASCII mockup in the spec above is a structural placeholder; the final card should be polished and recognizable as a blueprint artifact (think: the kind of card you'd actually want to see appear in a LinkedIn feed). A design pass is needed before Phase 1 implementation.

**3. The agent trading card is a download artifact, not a social broadcast.**  
The trading card (portrait, dark, holographic) is something the user downloads and keeps. Downloads use the existing `downloadSvg()` and `downloadPng()` functions — no new server endpoint. Social sharing from the agent card routes to the blueprint because: (a) the trading card is portrait-format and letterboxes on all platforms' `summary_large_image` layout, and (b) the blueprint — not the deployment — is the public-facing entity that makes sense to someone seeing it cold.

**4. Cases 1 and 2 share the same gate and the same endpoint.**  
The Share button is a thin client-side action that constructs a platform intent URL with the blueprint URL. What the platform crawler hits is the blueprint page — same HTML, same `og:image` pointing to the same badge endpoint. One badge endpoint, one visibility gate.

**5. On-demand generation, not pre-generation.**  
Pre-generating PNGs at creation time would require the backend to know about the Bun server (or a separate image service), adding coupling. On-demand generation is simpler, and the LRU + HTTP cache provides sufficient performance for crawlers, which are low-frequency.

**6. No OG tags in `root.tsx`.**  
Global fallback OG tags are intentionally omitted. Adding them would cause non-blueprint pages to unfurl with incorrect metadata. Per-page `meta()` exports are the correct scope.

**7. Satori requires inline styles.**  
Satori does not support Tailwind classes, CSS modules, or external stylesheets — only a flexbox-compatible subset of inline CSS. The `BlueprintCard` component MUST use inline style objects exclusively and is maintained separately from the equivalent browser UI component.
