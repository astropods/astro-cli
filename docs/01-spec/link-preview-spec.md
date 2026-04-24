# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

Three cases, one endpoint that matters.

**Cases 1 + 2 converge.** Whether a user clicks Share anywhere in the app (Case 1) or pastes a blueprint URL into LinkedIn, X, or Slack (Case 2), the result is identical: a visibility gate determines whether the blueprint card PNG unfurls. No separate social path exists for the agent badge — trading card downloads use existing client-side functions.

**Case 3 — Deployment URL.** Returns 404, no OG metadata, no unfurl. Intentional until FGAC.

---

## Problem Statement

Blueprint pages already emit OG tags, but `og:image` points to `${assetsBase}/avatars/${account}.jpg` — the same account avatar for every blueprint that account owns. There is no blueprint-specific image. Agent detail pages emit no OG metadata at all.

The gap: no server-side mechanism exists to render a PNG for a specific entity. SVG (what `astro-trading-card` produces) cannot be served as `og:image` — platforms require a raster image at that URL.

---

## Access Control

Platform crawlers are unauthenticated — session checks cannot reliably gate OG images. Three constraints follow from this:

1. **Blueprint visibility is the gate.** Public = badge renders + OG tags emitted. Private = Share button's social options disabled (tooltip: _"Make this blueprint public to share it"_), badge endpoint returns 404, no OG tags.
2. **Deployment URLs are dark.** No org membership check is possible without FGAC; unfurling deployments is deferred until then.
3. **FGAC future state.** Once FGAC exists, private-scope sharing can be unlocked for org members without changing the badge infrastructure.

---

## Goals

- **G1:** Public blueprint URLs unfurl with a blueprint-specific landscape PNG. Private blueprints: 404 from badge endpoint, no OG tags, Share button's social options disabled.
- **G2:** Share button (anywhere in app) produces the same unfurl as pasting the blueprint URL directly. Disabled with tooltip for private blueprints.
- **G3:** Share button pre-fills a platform message with the agent description and blueprint URL.
- **G4:** Trading card SVG and PNG downloads work via existing client-side functions — no new server endpoint.
- **G5:** Deployment URLs return no OG metadata.
- **G6:** PNGs are generated on demand server-side, cached in memory, cacheable at the HTTP layer.
- **G7:** No Go backend changes required.

## Non-Goals

- Agent badge as a social OG image.
- Deployment page OG tags (until FGAC).
- Private blueprint sharing (until FGAC).
- Pre-generated / CDN-uploaded PNGs.
- Animated or video cards.
- Changing the trading card visual design.

---

## Design

### Flow

```
Case 1: User clicks Share (anywhere in app)
Case 2: User pastes blueprint URL into LinkedIn / X / Slack
         |
         v
    blueprint public?
     |           \
    YES            NO --> 404, no unfurl; Share social options disabled
     |
     v
GET /badge/blueprint/:account/:name.png
  1. Fetch blueprint from API
  2. Visibility check -- 404 if private
  3. blueprintToCardProps()
  4. satori(BlueprintCard)   -- JSX -> SVG
  5. Resvg().render()        -- SVG -> PNG
  6. Return PNG (LRU cached)
     |
     v
Blueprint Card PNG (1200x630, landscape)
LinkedIn  ·  X  ·  Slack


Case 3: User pastes deployment URL anywhere
  -> 404 / Not Authorized, no OG tags, no unfurl
  (Until FGAC: no org membership check possible at OG layer)


Agent card downloads (already implemented, no new endpoint):
  downloadSvg()  -- client-side, astro-trading-card/browser
  downloadPng()  -- client-side, existing export
```

---

### Blueprint Badge Endpoint

**Route:** `GET /badge/blueprint/:account/:name.png`

| Scenario | Status |
|----------|--------|
| Public blueprint | `200 OK` — PNG, `Content-Type: image/png` |
| Private blueprint | `404` |
| Not found | `404` |
| API error | `502` |

**Headers on 200:**
```
Cache-Control: public, max-age=3600, stale-while-revalidate=86400
```

#### Visual Design

> **Design pass required before Phase 1.** The card should be visually distinctive and appealing — something you'd actually want to see in a LinkedIn feed, specific to blueprints. The layout below is a structural reference only.

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
| Icon | Rounded square, 80x80px; initials fallback |
| Title | Bold, ~28px, `#111827` |
| Description | ~18px, `#6b7280`, max 2 lines |
| Footer left | Deploy count |
| Footer right | Owner avatar (24px) + handle |
| Corner radius | 12px |
| Border | 1px, `#e5e7eb` |

#### Rendering

Satori converts the `BlueprintCard` JSX component to SVG; Resvg converts SVG to PNG. Fonts are loaded once at server startup.

```typescript
import satori from "satori";
import { Resvg } from "@resvg/resvg-js";

const font = readFileSync("path/to/Inter-Regular.ttf");
const fontBold = readFileSync("path/to/Inter-Bold.ttf");

async function handleBlueprintBadge(account: string, name: string): Promise<Response> {
  const res = await fetch(`${API_URL}/api/v1/blueprints/${account}/${name}`);
  if (!res.ok) return new Response(null, { status: res.status === 404 ? 404 : 502 });

  const blueprint = await res.json();
  if (!blueprint.public) return new Response(null, { status: 404 });

  const svg = await satori(BlueprintCard(blueprintToCardProps(blueprint)), {
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

`BlueprintCard` lives in `src/badge-blueprint.tsx`. Inline styles only — no Tailwind, no CSS modules.

#### Data Mapping

| CardProps field | Source |
|----------------|--------|
| `displayName` | `blueprint.display_name` (fallback: `blueprint.name`) |
| `description` | `blueprint.description` (clamped to 2 lines) |
| `avatarUrl` | `blueprint.avatar_url` (fallback: initials) |
| `deployCount` | `blueprint.deployment_count` |
| `ownerHandle` | `blueprint.account` |
| `ownerAvatarUrl` | `blueprint.account_avatar_url` |

---

### Open Graph Meta Tags

The `host` is derived from `new URL(request.url).host` in the loader — never hardcoded.

**Blueprint Detail page** (`BlueprintDetail.tsx`) already has a `meta()` export. Only the `ogImage` derivation changes. OG tags are omitted entirely for private blueprints.

| Property | Old | New |
|----------|-----|-----|
| `og:image` | `${assetsBase}/avatars/${account}.jpg` | `https://{host}/badge/blueprint/{account}/{name}.png` |
| `og:image:width` | — | `1200` |
| `og:image:height` | — | `630` |
| `twitter:image` | same as old `og:image` | same as new `og:image` |

**Agent detail page:** no OG tags. Deployment URLs are intentionally dark.

---

### Cache

| Property | Value |
|----------|-------|
| Key | `blueprint:${account}/${name}` |
| Value | PNG `Uint8Array` |
| Max entries | 500 |
| TTL | 1 hour |
| Package | `lru-cache` |

---

### Dependencies

| Package | Change |
|---------|--------|
| `@resvg/resvg-js` | Add |
| `satori` | Add |
| `lru-cache` | Add |

---

## In-App Share Flow

The Share button (agent card or anywhere in the app) has two sections: **download** and **social share**.

**Download** (always available, no visibility gate):

| Option | Implementation |
|--------|---------------|
| Download SVG | `downloadSvg()` — existing, client-side |
| Download PNG | `downloadPng()` — existing, client-side |

**Social share** (disabled for private blueprints — tooltip: _"Make this blueprint public to share it"_):

Opens the platform intent URL with the blueprint URL and a pre-filled message.

| Platform | Intent URL |
|----------|-----------|
| LinkedIn | `https://www.linkedin.com/sharing/share-offsite/?url={encodedBlueprintUrl}` |
| X | `https://x.com/intent/post?text={encodedMessage}&url={encodedBlueprintUrl}` |

Pre-filled messages:

| Platform | Text |
|----------|------|
| LinkedIn | `"Just deployed {displayName} on Astro using the {blueprint} blueprint. Check it out: {blueprintUrl}"` |
| X | `"Just deployed {displayName} on Astro using {blueprint} {blueprintUrl}"` |

---

## Files to Modify or Create

| File | Change |
|------|--------|
| `apps/astro-client/server.ts` | Add `/badge/blueprint/*` route before React Router handler |
| `apps/astro-client/src/badge-blueprint.tsx` | **New.** `BlueprintCard` JSX + `generateBlueprintBadgePng()` |
| `apps/astro-client/src/badge-cache.ts` | **New.** LRU cache instance |
| `apps/astro-client/src/pages/blueprints/BlueprintDetail.tsx` | Replace `ogImage` in loader; add width/height; omit OG tags if private |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add Share menu; wire existing download functions; add social share intent links; disable social options for private blueprints |
| `apps/astro-client/package.json` | Add `@resvg/resvg-js`, `satori`, `lru-cache` |

---

## Implementation Order

**Phase 1 — Blueprint badge + unfurl:**
1. Install `@resvg/resvg-js`, `satori`, `lru-cache`
2. Design pass on `BlueprintCard` visual
3. Implement `src/badge-blueprint.tsx` and `src/badge-cache.ts`
4. Add `/badge/blueprint/*` handler in `server.ts`
5. Verify: `curl http://localhost:3000/badge/blueprint/postman/release-note-helper.png > out.png`
6. Update `BlueprintDetail.tsx` loader: new `ogImage`, add width/height, omit OG tags if private
7. Verify with LinkedIn Post Inspector, Twitter Card Validator, Slack

**Phase 2 — Share menu:**
1. Add Share menu to `DeployedAgentDetail.tsx`
2. Wire Download SVG and Download PNG to existing functions
3. Wire social share to intent URLs with pre-filled message
4. Disable social options + tooltip for private blueprints
5. Verify on both platforms; confirm blueprint card unfurls

---

## Key Design Decisions

**1. Bun vs. Go for PNG generation.**  
Satori is a TypeScript library — Bun is the natural fit. However, the Go backend already has `oksvg` + `rasterx` at `internal/identitygen/raster.go` for avatar generation. **Investigate before Phase 1:** pipe a sample Satori SVG through the Go rasterizer. If output quality is acceptable, serve the badge from Go (unified origin, no new Bun dependencies). If not, proceed with Bun + Resvg.

**2. Blueprint card needs a creative pass.**  
The card should be polished and blueprint-specific — not a generic layout. The structural mockup in this spec is a placeholder. Design happens before Phase 1 implementation.

**3. Trading card is download-only.**  
Portrait format letterboxes on every platform's `summary_large_image`. More importantly, the blueprint — not the deployment — is the right public-facing entity for social sharing. Downloads use the existing client-side functions; no server endpoint needed.

**4. One gate, one endpoint.**  
The Share button constructs a platform intent URL with the blueprint URL. The crawler hits the blueprint page and reads `og:image`. Cases 1 and 2 are identical from the server's perspective.

**5. No OG tags in `root.tsx`.**  
Per-page `meta()` exports are the correct scope. Global fallback tags would cause non-blueprint pages to unfurl incorrectly.

**6. Satori requires inline styles.**  
No Tailwind, no CSS modules. `BlueprintCard` uses inline style objects only and is maintained separately from the browser UI component.
