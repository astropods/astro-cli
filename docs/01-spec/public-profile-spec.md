# Public Profile Page Spec

**Status:** Draft  
**Author:** Sohum Dalal  
**Date:** 2026-04-29

---

## Abstract

Every account gets a public profile page at `/:account`. Today it is sparse. This spec ships a full-featured profile: a persistent sidebar with identity and social presence, tabbed main content, inline editing (no separate settings page), and an explicit internal/external view toggle for owners. The design is driven by Taylor's `feature/profile-tabs` branch; this spec fills in the API surface, replaces mock data, and calls out all gaps.

---

## Problem Statement

The `/:account` page currently provides no meaningful public presence — no bio, no social links, no unified content browser. Owners have no way to curate how they appear publicly. There is no distinction between what an owner sees on their own profile vs what a visitor sees.

---

## Goals

- **G1:** Sidebar shows bio, social links (website, GitHub, X, LinkedIn, Astro domain), joined date, blueprint + agent counts, org memberships.
- **G2:** Explicit internal/external view toggle for owners — "internal" is the owner's full view, "external" previews exactly what a visitor sees.
- **G3:** Three tabs: Blueprints, Hearts (both views), Agents (internal view only).
- **G4:** All tabs have search, visibility filter (All / Public / Private), and sort — internal view only. External view has no visibility filter.
- **G5:** Inline editing: sidebar toggles an `isEditing` state in-place. No separate settings page.
- **G6:** Drag-to-reorder blueprints via `@dnd-kit` (internal view only). Order persists to `localStorage` for now.
- **G7:** Server persists bio and social links on the `accounts` table.
- **G8:** Early adopter badge awarded to the first 1000 accounts by `created_at`.
- **G9:** Hearts tab backed by real API — heart toggle + listing endpoint.
- **G10:** Org memberships in sidebar backed by real data.

## Non-Goals

- Blueprint pinning / spotlight (stub `pinned_at` exists; deferred).
- Blueprint order server-side persistence (localStorage acceptable for now).
- Hearts on deployments.
- Follower / following counts.
- Per-blueprint privacy toggle from the profile page.

---

## Internal vs External View

Owners see a toggle on their own profile: **Internal view** (default) ↔ **External view** ("View as visitor").

| | External view (visitor) | Internal view (owner) |
|---|---|---|
| Who sees it | All visitors + owner in preview mode | Owner only |
| Blueprints tab | Public blueprints only | All blueprints (public + private) |
| Visibility filter | Hidden | All / Public / Private |
| Agents tab | Hidden | Visible |
| Hearts tab | Visible | Visible |
| Drag-to-reorder | Hidden | Visible |
| Edit profile | Hidden | Inline editing in sidebar |
| "View as visitor" toggle | — | Shown |

`viewMode: "internal" | "external"` is client-side state, defaulting to `"internal"` for the authenticated owner. Visitors never see the toggle — they always get the external view.

---

## Access Control

| Viewer | Effective view |
|--------|---------------|
| Public visitor | External view, no toggle |
| Authenticated non-owner | External view, no toggle |
| Owner | Internal view by default; can toggle to external preview |

---

## Design

### Page Structure

Two-column layout. Sidebar (~288px, sticky) on the left; scrollable main content on the right.

**Sidebar:**
- `GradientGridWash` decorative background using account avatar colors
- Avatar, display name, handle
- Early adopter badge (if applicable — see below)
- Bio (omitted if empty, placeholder shown in edit mode)
- Social links: Astro domain, website, GitHub, X, LinkedIn — omit unset ones in view mode; show all inputs in edit mode
- Joined date
- Stats: blueprint count, agent count (agent count visible to owner in internal view only)
- Organizations: Astro logo + org avatar chips
- **Internal view:** "Edit profile" button → toggles `isEditing` in sidebar. In edit mode, all sidebar fields become inputs in-place. "Save" / "Cancel" at bottom.
- **External view / visitor:** sidebar is read-only

### Inline Editing

No navigation to `/settings/profile`. The sidebar owns all editing via `isEditing: boolean` state.

In edit mode:
- Display name → text input (`DISPLAY_NAME_MAX_LENGTH`)
- Bio → textarea (3 rows, non-resizable)
- Social links → inline inputs for website URL, X handle, LinkedIn URL, GitHub handle
- Avatar → click avatar to open existing `AvatarUploadDialog`
- "Save changes" / "Cancel" buttons at bottom of sidebar

Save dispatches two concurrent mutations:
- `PATCH /api/v1/me` — display name (existing)
- `PATCH /api/v1/accounts/:account` — bio + social fields (new endpoint)

Both call `refresh()` on success. On error: inline error message below the form, sidebar stays in edit mode.

---

### Tabs

#### Blueprints

- **External view:** public blueprints only; no visibility filter.
- **Internal view:** all blueprints; visibility filter shows All / Public / Private.
- Sort: Last modified (default), Name A–Z, Most deployed, Most hearted.
- Pagination: 9 per page, "Load more".
- **Drag-to-reorder** (internal view only, `@dnd-kit`): clicking "Customize order" resets visibility filter to "All" and clears search/sort — reordering must operate on the full set. Disabled while any filter is active after that. Order saved to `localStorage[blueprint-order-${account}]`. Grip icon visible on hover in normal mode, always visible in reorder mode.
- Empty states: "No blueprints published yet" / "No blueprints match your filters".

#### Agents (internal view only)

Tab is not rendered at all in external view or for visitors.

- Shows all deployments for the account.
- Search by name / display name.
- Sort: Last modified (default), Name A–Z.
- Visibility filter: All / Public / Private (deployments are private by default; filter still present for consistency).
- Empty states: "No agents deployed yet" / "No agents match your search".

#### Hearts

Visible in both views.

- Blueprints the account has hearted, newest first.
- `BlueprintCard` with red heart overlay.
- Sort: Last hearted (default), Most deployed, Most hearted, Name A–Z.
- Visibility filter: All / Public / Private (external view locks to Public).
- Empty states: "No hearted blueprints yet" / "No hearted blueprints match your filters".

---

### Early Adopter Badge

Awarded to accounts whose `account_number` ≤ 1000.

Add a `account_number` serial column to `accounts`:

```sql
ALTER TABLE accounts ADD COLUMN account_number serial;
```

This auto-increments on insert. Existing rows get backfilled by `created_at` order via a one-time migration. The badge renders in the sidebar when `account_number <= 1000`. No user action needed; awarded at account creation time.

`GET /api/v1/accounts/:account` returns `account_number` in the response. Client renders the badge if `account_number != null && account_number <= 1000`.

---

## Server Changes

### Schema

Extend `accounts` table (all nullable, backward-compatible):

| Column | Type | Notes |
|--------|------|-------|
| `account_number` | `serial` | Auto-incrementing; backfill existing rows by `created_at` |
| `bio` | `varchar(500)` | Nullable |
| `website` | `varchar(255)` | Nullable, URL |
| `x_handle` | `varchar(50)` | Nullable, username only |
| `linkedin_url` | `varchar(255)` | Nullable, URL |
| `github_handle` | `varchar(50)` | Nullable, username only |

`astro_domain` = account slug, no new column needed.

Update `sql/astro-server/schema.sql` (Atlas diffs and applies only changed columns).

### Migration cost

Low. The existing `PATCH /api/v1/accounts/:account` handler already exists — it currently updates `display_name`. Extend it to accept the new fields. Add `accountStore.UpdateProfile()` (~15 lines). Extend `AccountResponse` to include new fields + `account_number`. Total: ~65 lines of Go + the schema change.

### Updated endpoints

**`GET /api/v1/accounts/:account`** — extend response to include `bio`, `website`, `x_handle`, `linkedin_url`, `github_handle`, `account_number`.

**`PATCH /api/v1/accounts/:account`** (already exists) — extend to accept `{ bio?, website?, x_handle?, linkedin_url?, github_handle? }`. Member-only. Validate max lengths + URL format for `website` and `linkedin_url`.

### New endpoints

**`GET /api/v1/accounts/:account/orgs`** — public. Returns org slugs + avatar URLs the account belongs to. Replaces `LOCAL_ORG_OVERRIDES`.

**`POST /api/v1/agents/:account/:name/heart`** — authenticated only. Toggle heart. Returns `{ hearted: bool, count: int }`.

**`GET /api/v1/accounts/:account/hearts`** — public. Paginated list of blueprints hearted by the account. Returns `{ items: AgentSummary[], next_cursor?: string }`.

Blueprint list responses should include `heart_count` (denormalized counter on `agents` table).

---

## Client Changes

### View mode state

`viewMode: "internal" | "external"` lives in the `AccountProfile` page component. Default: `"internal"` if owner, otherwise permanently `"external"`. The toggle renders only for the owner in internal view.

### Modified files

| File | Change |
|------|--------|
| `src/pages/AccountProfile.tsx` | Add `viewMode` state, inline editing in sidebar, tabs + filter logic per view |
| `src/lib/api.ts` | Add `bio`, `website`, `x_handle`, `linkedin_url`, `github_handle`, `account_number` to `AccountPublic`; add `updateAccountProfile`, `toggleHeart`, `listHeartedBlueprints`, `getAccountOrgs` |
| `src/api/queries/accounts.ts` | Add `useUpdateAccountProfile()`, `useAccountOrgs()`, `useHeartedBlueprints()` |
| `src/components/BlueprintCard.tsx` | Add `heartCount` prop |
| `src/routes.ts` | No new routes (inline editing, no `/settings/profile`) |

### Mock / placeholder cleanup

| Location | Replace with |
|----------|-------------|
| `AccountProfile.tsx` — `MOCK_HEARTED` | `useHeartedBlueprints()` |
| `AccountProfile.tsx` — `LOCAL_ORG_OVERRIDES` | `useAccountOrgs()` |
| `AccountProfile.tsx` — `LOCAL_PROFILE_OVERRIDES` | Account query response fields |
| Social links `href="#"` | Construct real hrefs from account fields |

---

## PR Breakdown

7 PRs end to end. Server PRs can be reviewed in parallel with early client PRs since the client mocks until the real API lands.

---

**PR 1 — Server: profile fields + orgs endpoint** *(Go only)*
- Schema: `account_number` serial + backfill by `created_at`, `bio`, `website`, `x_handle`, `linkedin_url`, `github_handle` columns
- Extend `AccountResponse` to include new fields + `account_number`
- Extend `PATCH /api/v1/accounts/:account` to accept bio + social fields; add `accountStore.UpdateProfile()`
- New `GET /api/v1/accounts/:account/orgs`
- **Reviewable alone.** Pure Go + schema. No client changes.

---

**PR 2 — Client: sidebar + inline editing** *(depends on PR 1)*
- Redesigned sidebar with real profile data from account query
- `isEditing` toggle: all sidebar fields flip to inputs in-place
- Save dispatches `PATCH /api/v1/me` + `PATCH /api/v1/accounts/:account` concurrently
- Early adopter badge from `account_number`
- Wire org memberships via `useAccountOrgs()`
- Wire social link hrefs; remove `LOCAL_PROFILE_OVERRIDES` + `LOCAL_ORG_OVERRIDES`
- **Reviewable alone.** Sidebar only — no tabs, no view toggle.

---

**PR 3 — Client: view toggle + Blueprints + Agents tabs** *(depends on PR 2)*
- `viewMode: "internal" | "external"` state + toggle button
- Blueprints tab: search, visibility filter (internal only), sort, pagination; public-only in external view
- Agents tab: internal view only, search + sort
- Apply per-tab filter/visibility rules based on `viewMode`
- **Reviewable alone.** No drag, no hearts.

---

**PR 4 — Client: drag-to-reorder** *(depends on PR 3)*
- `@dnd-kit` integration on Blueprints tab (already in Taylor's branch)
- "Customize order" resets visibility filter to All + clears search/sort
- Grip handles, reorder mode state machine (`idle` → `editing` → `saved`)
- `localStorage[blueprint-order-${account}]` persistence
- **Reviewable alone.** Self-contained interaction layer on top of PR 3.

---

**PR 5 — Server: Hearts backend** *(Go only, can be parallelized with PR 3/4)*
- `agent_hearts` table: `(account_id, agent_id, created_at)`, unique constraint
- Denormalized `heart_count` on `agents` table; incremented/decremented on toggle
- `POST /api/v1/agents/:account/:name/heart` — toggle, returns `{ hearted: bool, count: int }`
- `GET /api/v1/accounts/:account/hearts` — paginated, public
- Add `heart_count` to blueprint list response
- **Reviewable alone.** Pure Go.

---

**PR 6 — Client: Hearts tab** *(depends on PR 3 + PR 5)*
- Hearts tab wired to `useHeartedBlueprints()`
- Heart toggle button on `BlueprintCard` (authenticated users)
- `heartCount` prop on `BlueprintCard`
- Remove `MOCK_HEARTED`
- Search, visibility filter (internal only), sort
- **Reviewable alone.** Last tab; completes the feature.

---

### Summary

| PR | Scope | Depends on |
|----|-------|-----------|
| 1 | Server: profile fields + orgs | — |
| 2 | Client: sidebar + inline editing | PR 1 |
| 3 | Client: view toggle + Blueprints + Agents | PR 2 |
| 4 | Client: drag-to-reorder | PR 3 |
| 5 | Server: Hearts backend | — |
| 6 | Client: Hearts tab | PR 3, PR 5 |

Critical path: PR 1 → PR 2 → PR 3 → PR 4. PR 5 can be cut in parallel. PR 6 gates on both PR 3 and PR 5.

---

## Key Design Decisions

**1. Inline editing, no separate settings page.**  
Navigating away to `/settings/profile` breaks the mental model of "this is my public profile." Toggling `isEditing` in the sidebar lets owners edit and immediately see the result in context. `/settings/account` keeps email, username, and auth — non-profile concerns.

**2. Internal/external toggle instead of implicit isMember.**  
An explicit `viewMode` makes the owner's experience intentional. They can preview exactly what visitors see. It also cleanly separates which data and which UI elements render in each mode, rather than scattering `isMember` conditionals throughout.

**3. Agents tab is internal view only.**  
Deployments are private infrastructure. Visitors have no reason to browse another user's running agents. Removing it from the external view removes the need for any visibility logic on that tab.

**4. account_number serial for early adopter.**  
UUIDs and `created_at` timestamps are non-deterministic for ordering purposes in application logic. A serial column is cheap, backfills cleanly, and gives a stable, queryable rank. First 1000 accounts by `created_at` get the badge via a one-time backfill.

**5. Visibility filter is internal view only.**  
External view has no visibility filter — visitors always see public content only. Entering "customize order" mode resets the filter to "All" so the owner reorders the full set, not a filtered subset.

**6. @dnd-kit for drag-to-reorder.**  
Already used in Taylor's branch. No new dependency. Internal view only, disabled while filters are active. localStorage persistence is sufficient for v1; server-side order is a clean follow-on.

**7. Full-stack cost is low.**  
No new table. Five nullable columns on `accounts` + ~65 lines of Go. The `PATCH /api/v1/accounts/:account` endpoint already exists.
