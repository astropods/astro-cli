# Link Preview (Social Unfurl) Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-24

---

## Abstract

Three cases, one endpoint that matters.

**Cases 1 + 2 converge.** Whether a user clicks Share anywhere in the app (Case 1) or pastes a blueprint URL into LinkedIn, X, or Slack (Case 2), the result is identical: a visibility gate determines whether the blueprint card PNG unfurls. No separate social path exists for the agent badge — trading card downloads use existing client-side functions.

**Case 3 — Deployment URL.** Deployment pages render normally; no OG tags are emitted; no badge endpoint exists for deployments until FGAC.

---

## Problem Statement

Blueprint pages already emit OG tags, but `og:image` points to `${assetsBase}/avatars/${account}.jpg` — the same account avatar for every blueprint that account owns. There is no blueprint-specific image. Agent detail pages emit no OG metadata at all.

The gap: no server-side mechanism exists to render a PNG for a specific entity. SVG (what `astro-trading-card` produces) cannot be served as `og:image` — platforms require a raster image at that URL.

---

## Access Control

Platform crawlers are unauthenticated — session checks cannot reliably gate OG images. Three constraints follow from this:

1. **Blueprint visibility is the gate.** `visibility == "public"` = badge renders + OG tags emitted. Private, draft, or name-reserved = Share button's social options disabled (tooltip: _"Make this blueprint public to share it"_), badge endpoint returns 404, no OG tags.
2. **Deployment URLs are dark.** No org membership check is possible without FGAC; unfurling deployments is deferred until then.
3. **FGAC future state.** Once FGAC exists, private-scope sharing can be unlocked for org members without changing the badge infrastructure.

---

## Goals

- **G1:** Public blueprint URLs unfurl with a blueprint-specific landscape PNG. Private/draft/name-reserved blueprints: 404 from badge endpoint, no OG tags, Share button's social options disabled.
- **G2:** Share button (anywhere in app) produces the same unfurl as pasting the blueprint URL directly. Disabled with tooltip for private blueprints.
- **G3:** Share button pre-fills a platform message with the agent description and blueprint URL.
- **G4:** Trading card SVG and PNG downloads work via existing client-side functions — no new server endpoint.
- **G5:** Deployment URLs return no OG metadata.
- **G6:** PNGs are generated on demand server-side, cached in memory, cacheable at the HTTP layer.
- **G7:** Served from astro-server, reusing the identitygen rasterizer.

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
GET /badge/agents/:account/:name.png   (served from astro-server)
  1. Validate account + name format (400 on violation)
  2. agentindex.Get(acct.ID, name) -- no HTTP hop
     + metrics, avatar bytes in parallel
  3. 404 if Visibility != "public" || len(Versions) == 0 || NameReserved
  4. Map agent fields to card data
  5. Base64-encode avatar bytes; inject into SVG template
  6. identitygen rasterizer  -- SVG -> PNG
  7. Return PNG (in-memory cached, singleflight-guarded)
     |
     v
Blueprint Card PNG (1200x630, landscape)
LinkedIn  ·  X  ·  Slack


Case 3: User pastes deployment URL anywhere
  -> Deployment page renders normally
  -> No OG tags emitted, no badge endpoint, no unfurl
  (Until FGAC: no org membership check possible at OG layer)


Agent card downloads (already implemented, no new endpoint):
  downloadSvg()  -- client-side, astro-trading-card/browser
  downloadPng()  -- client-side, existing export
```

---

### Blueprint Badge Endpoint

**Route:** `GET /badge/agents/:account/:name.png` (astro-server)

| Scenario | Status |
|----------|--------|
| Public blueprint | `200 OK` — PNG, `Content-Type: image/png` |
| Private blueprint | `404` |
| Draft blueprint (`versions.length == 0`) | `404` |
| Name-reserved slot (`name_reserved == true`) | `404` |
| Invalid name format | `400` |
| Not found | `404` |
| Rasterization error | `502` |

**Headers on 200:**
```
Cache-Control: public, max-age=3600, stale-while-revalidate=86400
```

**Headers on 404/400:**
```
Cache-Control: public, max-age=300
```

> LinkedIn caches OG images for ~7 days with no self-serve invalidation. The `/badge/agents/` path is effectively permanent — any future rename breaks unfurls per-entity for up to a week.

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

> **Description clamping:** pre-truncate with ellipsis in Go before injecting into the SVG template. Confirm character budget against the design before Phase 1.

#### Rendering

`internal/identitygen/raster.go` already converts SVGs to PNG via oksvg + rasterx (used today for avatar generation). The badge handler extends this path: render a Go SVG template for the blueprint card, then pass it through a new `rasterizeSVGToPNG` variant alongside the existing JPEG function.

Fonts are embedded at compile time via `//go:embed` under `internal/badge/fonts/` — no runtime I/O, no Docker path issues. No new Bun-side dependencies (Satori, Resvg, lru-cache) are introduced.

The SVG template lives in `internal/badge/template.go` — pure Go `text/template`. Avatar images are fetched with a **500ms timeout** from the avatar store, base64-encoded, and injected as `data:image/jpeg;base64,...` `<image>` elements. Any fetch failure (timeout, missing, or error) falls back to inline SVG initials via `identitygen.GenerateIdentity` — slow avatar CDN must not stall badge requests.

> **Pre-Phase 1 check:** verify oksvg renders `<image>` elements with data URI sources. If not supported, use `identitygen.GenerateIdentity` for both avatar slots unconditionally.

#### Data Mapping

Real shape comes from `AgentResponse` (`handlers/agents.go`) and `AgentMetrics`. The account avatar is not on the agent response — it is fetched via `accountStore.GetByName`, which is already wired into `GetAgent` and is a free join on the Go path.

| Card field | Go source |
|-----------|-----------|
| `displayName` | `versions[0].agent_card.display_name` (fallback: `agent.Name`) |
| `description` | `versions[0].agent_card.description` (pre-truncated, clamped to 2 lines) |
| `agentAvatarBytes` | `avatarStore.ReadAgentAvatar(ctx, account, name)` — fallback: `identitygen.GenerateIdentityJPEG` |
| `deployCount` | `agent.Metrics.DeployCount` |
| `ownerHandle` | `account` (URL param) |
| `ownerAvatarBytes` | `avatarStore.ReadAvatar(ctx, account)` — fallback: `identitygen.GenerateIdentityJPEG` |

#### Handler Logic

The handler calls `agentindex.Get(acct.ID, name)` directly — no HTTP hop to the agent API. This mirrors `GetAgent`'s visibility + membership checks so FGAC is a zero-change upgrade later.

**Input validation** (before any DB call):
- Account: must pass `ValidateAccountName` (already exists in the codebase)
- Agent name: allowlist `^[a-z0-9][a-z0-9._-]*$`, max 64 chars → `400` on violation

**404 conditions** (in order):
1. Account not found
2. Agent not found
3. `agent.Visibility != "public"`
4. `len(agent.Versions) == 0` (draft)
5. `agent.NameReserved == true`

#### Concurrency

Rasterization is CPU-heavy on any stack. Wrap the rasterizer call in:
- A `singleflight.Group` keyed on cache key — collapses crawler stampedes for the same agent
- A semaphore of `min(GOMAXPROCS, 4)` concurrent renders — excess requests queue with a hard deadline; return `429` if deadline exceeded

---

### Open Graph Meta Tags

The `origin` is derived from `new URL(request.url).origin` in the loader — never hardcoded.

**Blueprint Detail page** (`apps/astro-client/src/pages/BlueprintDetail.tsx`) already has a `meta()` export. Only the `ogImage` derivation changes. OG tags are omitted when `visibility === "private"`, `versions.length === 0` (draft), or `name_reserved === true`.

| Property | Old | New |
|----------|-----|-----|
| `og:image` | `${assetsBase}/avatars/${account}.jpg` | `${origin}/badge/agents/{account}/{name}.png` |
| `og:image:width` | — | `1200` |
| `og:image:height` | — | `630` |
| `twitter:image` | same as old `og:image` | same as new `og:image` |

> `origin` = `new URL(request.url).origin` in the loader. `/badge/agents/` is proxied to astro-server via `PROXY_PREFIXES` in `apps/astro-client/server.ts` — a one-line client change.

**Agent detail page:** no OG tags. Deployment URLs are intentionally dark.

---

### Cache

`hashicorp/golang-lru/v2` LRU in `internal/badge/cache.go`. Cache key includes `updated_at` for implicit invalidation on any agent edit — no manual busting, no staleness after a description or avatar change.

| Property | Value |
|----------|-------|
| Key | `sha256(account + "/" + name + "/" + updatedAt)` |
| Value | PNG `[]byte` |
| Max entries | 500 |
| TTL | 1 hour |

A `singleflight.Group` (keyed on the same hash) wraps the render path, collapsing concurrent cache-miss requests for the same agent into one render.

---

### Dependencies

No new client-side npm packages. Go-side:

| Package | Status |
|---------|--------|
| `github.com/srwiley/oksvg` | Already in `go.mod` |
| `github.com/srwiley/rasterx` | Already in `go.mod` |
| `golang.org/x/image/draw` | Already in `go.mod` |
| `image/png`, `text/template`, `embed` | stdlib |
| `golang.org/x/sync/singleflight` | Already in `go.mod` |
| `github.com/hashicorp/golang-lru/v2` | **Add** |

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
| `apps/astro-client/server.ts` | Add `/badge` to `PROXY_PREFIXES` (one line) |
| `apps/astro-server/handlers/badge.go` | **New.** `GetBlueprintBadge` handler — input validation, 404 conditions, cache lookup, render dispatch |
| `apps/astro-server/internal/badge/template.go` | **New.** SVG template for blueprint card |
| `apps/astro-server/internal/badge/fonts/` | **New.** Embedded Inter TTFs (`//go:embed`) |
| `apps/astro-server/internal/badge/cache.go` | **New.** LRU + singleflight wrapper |
| `apps/astro-server/internal/identitygen/raster.go` | Add `rasterizeSVGToPNG` alongside existing JPEG variant (~30 lines + test) |
| `apps/astro-server/main.go` | Register unauthenticated `/badge/agents/:account/:name.png` route |
| `apps/astro-client/src/pages/BlueprintDetail.tsx` | New `ogImage` in loader; add `og:image:width/height`; omit OG tags when private, draft, or name-reserved |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Add Share menu; wire download functions; add social share intent links; disable social options for private blueprints |

---

## Implementation Order

**Phase 1 — Blueprint badge + unfurl:**
1. Design pass on blueprint card SVG template
2. Add `rasterizeSVGToPNG` next to existing JPEG variant in `identitygen/raster.go` (+ test)
3. Implement `internal/badge/` package: SVG template, `embed.FS` fonts, `golang-lru` + `singleflight`
4. Implement `handlers/badge.go` (`GetBlueprintBadge`), mirroring `GetAgent`'s visibility + membership + draft checks
5. Register unauthenticated route in `main.go`
6. Add `/badge` to `PROXY_PREFIXES` in `apps/astro-client/server.ts`
7. Smoke: `curl http://localhost:3000/badge/agents/postman/release-note-helper.png > out.png`
8. Update `BlueprintDetail.tsx` loader: new `ogImage`, add `og:image:width/height`, omit OG tags when `visibility === "private"` or `versions.length === 0` or `name_reserved`
9. Verify with LinkedIn Post Inspector, X Card Validator, Slack

**Phase 2 — Share menu:**
1. Add Share menu to `DeployedAgentDetail.tsx`
2. Wire Download SVG and Download PNG to existing functions
3. Wire social share to intent URLs with pre-filled message
4. Disable social options + tooltip for private blueprints
5. Verify on both platforms; confirm blueprint card unfurls

---

## Key Design Decisions

**1. Go for PNG generation.**  
`internal/identitygen/raster.go` already rasterizes SVGs via oksvg + rasterx. Extending it to accept a blueprint SVG template is ~30 lines. The badge endpoint lives in astro-server (`GET /badge/agents/:account/:name.png`); astro-client proxies via `PROXY_PREFIXES`. This eliminates Satori, Resvg, and lru-cache from the client bundle and consolidates raster logic in one place. astro-server already has structured logging and metrics — observability on the badge endpoint is trivial on the Go path.

**2. Blueprint card needs a creative pass.**  
The card should be polished and blueprint-specific — not a generic layout. The structural mockup in this spec is a placeholder. Design happens before Phase 1 implementation.

**3. Trading card is download-only.**  
Portrait format letterboxes on every platform's `summary_large_image`. More importantly, the blueprint — not the deployment — is the right public-facing entity for social sharing. Downloads use the existing client-side functions; no server endpoint needed.

**4. One gate, one endpoint.**  
The Share button constructs a platform intent URL with the blueprint URL. The crawler hits the blueprint page and reads `og:image`. Cases 1 and 2 are identical from the server's perspective.

**5. No OG tags in `root.tsx`.**  
Per-page `meta()` exports are the correct scope. Global fallback tags would cause non-blueprint pages to unfurl incorrectly.
