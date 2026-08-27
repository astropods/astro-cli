# Account profile page

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

Every account gets a public profile page at `/:account`: a sidebar with
identity and social presence, tabbed content (Blueprints, Agents, Hearts),
inline editing, and an internal/external view split between owners/members
and visitors. This doc supersedes
[`01-spec/public-profile-spec.md`](../01-spec/public-profile-spec.md), whose
schema and some tab behavior describe the original design, not what shipped.

## Schema

Profile fields do **not** live on the `accounts` table itself. They live in a
separate 1:1 `account_profile` table (`sql/astro-server/schema.sql`):

```sql
CREATE TABLE public.account_profile (
    account_id uuid NOT NULL,
    account_number serial,
    bio varchar(500),
    location varchar(100),
    local_timezone varchar(50),
    pronouns varchar(50),
    website varchar(255),
    social_links text[] NOT NULL DEFAULT '{}',
    blueprint_order text[] NOT NULL DEFAULT '{}',
    CONSTRAINT account_profile_pkey PRIMARY KEY (account_id),
    CONSTRAINT account_profile_account_id_fkey FOREIGN KEY (account_id)
        REFERENCES public.accounts(id) ON DELETE CASCADE
);
```

`account.Store.Create` inserts an empty `account_profile` row (`social_links
= '{}'`) in the same transaction as the account, which is what assigns
`account_number` at registration time (a `serial`, not a backfilled column —
there's no separate migration step per account). Every account read
(`GetByName`, `GetByID`, `GetByWorkOSOrganizationID`, `GetOrgAccountsForUser`)
`LEFT JOIN`s `account_profile` and `COALESCE`s the array columns to `'{}'`.

There is no per-platform `x_handle`/`linkedin_url`/`github_handle` set of
columns. Social presence is one generic `social_links text[]` column, capped
at 4 entries, validated only for length (255 chars) server-side — the client
infers the platform from each URL's hostname (see
[Social links rendering](#social-links-rendering) below). There's also no
separate `astro_domain` column: an entry starting with `@` is treated as an
Astro handle client-side (`/lib/social-links.tsx`) and links to `/:handle`.

`agent_hearts` is keyed `(account_id, agent_name, user_id)`, not `(account_id,
agent_id, created_at)`:

```sql
CREATE TABLE public.agent_hearts (
    account_id uuid NOT NULL,
    agent_name text NOT NULL,
    user_id text NOT NULL,
    created_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT agent_hearts_pkey PRIMARY KEY (account_id, agent_name, user_id),
    CONSTRAINT agent_hearts_agent_fkey FOREIGN KEY (account_id, agent_name)
        REFERENCES public.agents(account_id, name) ON UPDATE CASCADE ON DELETE CASCADE
);
```

`heart_count` is **not** a denormalized counter on `agents`. Every read path
(`heartstore.BulkCount`, `heartstore.Info`, the inline `COUNT(*)` in
`ListHearted`) computes it live with a `COUNT(*)` against `agent_hearts` at
request time.

## Server API

All handlers live in `apps/astro-server/handlers/accounts.go` and
`handlers/hearts.go`, backed by `internal/account.AccountStore` and
`internal/heartstore.Store`.

| Endpoint | Handler | Auth | Notes |
|---|---|---|---|
| `GET /api/v1/accounts/:account` | `GetAccount` | Public | Returns `AccountResponse`: profile fields, `account_number`, `social_links`, `blueprint_order`, `allowed_clusters`, and (personal accounts only) an `owner` block from WorkOS. |
| `PATCH /api/v1/accounts/:account` | `UpdateAccount` | `org:manage` permission (owner, or org admin/owner; a personal account's sole member holds this implicitly) | Pointer/PATCH semantics: a field omitted from the JSON body leaves the stored value unchanged; an explicit empty string clears it. Validates `bio` ≤160 chars (tighter than the column's 500), `location`/`pronouns` ≤100/50, `website` as an `http(s)` URL ≤255 chars, `social_links` ≤4 entries of ≤255 chars each, `blueprint_order` ≤200 entries. |
| `GET /api/v1/accounts/:account/orgs` | `GetAccountOrgs` | Public | Org accounts the personal account's first member belongs to. 404s if `:account` isn't a personal account. No visibility filtering yet — a `TODO` in the handler notes public/private org filtering is unbuilt. |
| `POST /api/v1/agents/:account/:name/heart` | `ToggleHeart` | Any authenticated user (no account-membership check) | Atomically flips the caller's heart row and returns the new `{hearted, heart_count}` in one query (`heartstore.Toggle`'s CTE). |
| `GET /api/v1/accounts/:account/hearts` | `ListHearted` | Public | Cursor-paginated (`created_at` cursor, default page 20, max 100). 404s if `:account` isn't personal. Hardcodes `a.visibility = 'public'` and `a.archived_at IS NULL` — it never returns private blueprints, for any viewer, including the account owner. |

`UpdateAccount`'s route group requires `org:manage`, not literal ownership —
any org admin can edit an org's profile, matching the account-wide
authorization model rather than a profile-specific owner check.

## Internal vs external view

The client derives view state from account membership plus a URL query
param, not a persisted or session-scoped toggle (`ProfileLayout.tsx`):

```
isAdminView      = (isSelf || isOrgAdmin) && !isVisitorMode   // edit button, "View as visitor" link
isInternalView   = (isSelf || isOrgMember) && !isVisitorMode  // private blueprints, visibility filter, Agents tab
```

- `isVisitorMode` is `searchParams.has("visitor")` — visiting `/:account?visitor`
  forces the external view for anyone, including the owner.
- **Any org member** (not just an org admin/owner) gets the internal view for
  an organization account — they see private blueprints and the Agents tab.
  Only an org admin/owner (or the personal-account owner) gets the edit
  button and the "View as visitor" link. The spec's owner-only framing
  doesn't extend to org accounts as shipped.
- "View as visitor" (`ProfileLayout.tsx`) is a `target="_blank"` link to
  `/:account?visitor`, not an in-place state toggle — it opens the external
  view in a new tab rather than switching the current one.
- A public visitor or authenticated non-member always has `isInternalView =
  isAdminView = false`; there's no toggle rendered for them.

| | External view | Internal view |
|---|---|---|
| Who | Visitors, non-members, anyone on `?visitor` | Owner (personal) / any org member, not on `?visitor` |
| Blueprints tab | Public only | All; visibility filter (All/Public/Private) |
| Agents tab | Hidden | Visible, no visibility filter (see below) |
| Hearts tab | Visible, public blueprints only (API-enforced) | Visible, same API-enforced public-only result |
| Drag-to-reorder | Hidden | Visible (owner/admin only — `canManage`) |
| Edit profile / "View as visitor" | Hidden | Visible to owner/admin only |

## Tabs

Tab visibility and counts are computed in `ProfileLayout.tsx`; each tab owns
its own filter/sort state, lifted to `ProfileLayout` so "Customize order" can
reset it.

### Blueprints

- External view: `rawBlueprints.filter(bp => bp.visibility === "public")`.
  Internal view: all blueprints, then optionally narrowed by the visibility
  dropdown (All/Public/Private) — internal-only.
- Sort options are `newest` (default), `name`, `deployed` (most deployed).
  **There is no "Most hearted" sort** — the spec's fourth option was never
  built.
- No pagination: `useAccountBlueprints` fetches the full list in one
  unpaginated request (server caps at 100). There's no "9 per page / Load
  more" — the whole set renders and the grid just grows.
- "Customize order" (`handleEnterReorder`) resets search to `""`, visibility
  to `all`, and sort to `newest` before entering reorder mode, so dragging
  always operates on the complete, unfiltered set, matching the spec's
  design intent.

### Agents (internal view only)

Rendered only when `isInternalView` (any org member, or the personal owner)
and not in visitor mode; hidden entirely otherwise.

- Search by name/display name; sort is `modified` (default) or `name`.
- **No visibility filter.** The spec called for an All/Public/Private
  dropdown "for consistency" even though deployments are private by default;
  it was never built — `AgentsTab` only takes `search`/`sort` props.

### Hearts

Rendered for personal accounts only (`!isOrg` — org accounts have no Hearts
tab; hearting an org's blueprints isn't modeled per-org).

- Cursor-based pagination with explicit Previous/Next buttons (not
  infinite-scroll "Load more"), backed by `useHeartedBlueprints`.
- Client-side search only (substring match on name within the current page).
- **No sort control and no visibility filter** in the UI — the spec's "Last
  hearted / Most deployed / Most hearted / Name A–Z" sort and "All / Public /
  Private (external locked to Public)" filter were never built. This lines
  up with the server: `ListHearted` always filters to public, non-archived
  blueprints only, for every caller including the account owner, so an
  internal-view "all" filter would have nothing extra to show anyway.
- Heart toggling on a card is optimistic (`HeartsTab`'s local `Map` flips
  immediately, reverted on mutation error) and only rendered when
  `isOwner` — visitors and non-owners see the heart count but no toggle
  button on this tab.

## Inline editing

`ProfileLayout` swaps the sidebar between `ProfileViewSidebar` and
`ProfileEditSidebar` on a local `editOpen` boolean — there's no
`/settings/profile` route, matching the spec's design goal.

`ProfileEditSidebar.handleSave` fires up to two concurrent mutations:
- Display name: `useUpdateAccountDisplayName` (org) or `useUpdateProfile`
  (`PATCH /api/v1/me`, personal) — same split as before this feature.
- Everything else: `useUpdateAccountProfile` → `PATCH /api/v1/accounts/:account`
  with `bio`, `location`, `website`, `social_links` (empty strings filtered
  out), and, for personal accounts only, `pronouns` **and `local_timezone:
  ""`**.

Avatar upload reuses the existing `AvatarUploadDialog`, opened by clicking
the avatar.

**Real code issue:** `local_timezone` is a real column, round-tripped by
`GetAccount`/`AccountResponse`, and accepted by `PATCH`, but there is no
input for it anywhere in `ProfileEditSidebar` or any other UI, and it's
never displayed in `ProfileSidebarShell`. Every personal-account profile save
sends `local_timezone: ""`, which — because `UpdateAccount`'s pointer
semantics treat a present empty string as "set to empty," not "leave
unchanged" — silently clears any value in that column on every single save.
The field is effectively dead and self-erasing. Not fixed as part of this
doc pass; flagged for whoever next touches `ProfileEditSidebar.tsx`.

### Social links rendering

`social_links` is 4 free-text slots. `detectSocialLink`
(`apps/astro-client/src/lib/social-links.tsx`) infers the icon/label at
render time from the value:
- `@handle` → treated as an Astro profile link, links to `/:handle`.
- `github.com/...`, `linkedin.com/...` (or `/in/...`), `x.com`/`twitter.com`
  → platform icon, label extracted from the path.
- Anything else parseable as a URL → generic globe icon, hostname as label.

## Drag-to-reorder

**Server-side persistence has shipped.** The spec's non-goal ("Blueprint
order server-side persistence (localStorage acceptable for now)") is
superseded: there is no `localStorage` use anywhere under
`pages/AccountProfile` or `components/account-profile`. Saving a reorder
(`ProfileLayout.handleSaveReorder`) calls `useUpdateAccountProfile` with
`{ blueprint_order: names }`, which round-trips through the `account_profile`
table via `UpdateAccount`/`AccountStore.UpdateProfile`, and the sidebar seeds
its custom order from `data.blueprint_order` on load (`useEffect` in
`ProfileLayout`).

Reorder mode is a three-state machine (`idle → editing → saved → idle`) via
`@dnd-kit`:
1. "Customize order" clears filters (see Blueprints tab above) and enters
   `editing`.
2. Dragging updates local state only (`BlueprintsTab`'s `localOrder`).
3. "Save changes" optimistically applies the new order, sets `saved`, fires
   the mutation, and falls back to the previous order and re-enters
   `editing` on mutation error. `saved` auto-resets to `idle` after 1.5s on
   success.

## Early adopter badge

`ProfileViewSidebar` renders `EarlyAdopterBadge` when `!isOrg &&
data.account_number != null && data.account_number <= 1000` — org accounts
never get the badge, matching the spec. `account_number` is the
`account_profile.account_number` serial, assigned once at account creation
(see [Schema](#schema)); there's no separate backfill mechanism to inspect,
since new accounts get their number transactionally at insert.

## Known gaps / real issues found while writing this doc

Not fixed here — reported for whoever next touches these areas:

- **`local_timezone` self-clearing bug** — see [Inline editing](#inline-editing)
  above.
- **`GetAccountOrgs` has no visibility filtering** — the handler's own `TODO`
  notes that an authenticated owner should see all org memberships and an
  unauthenticated visitor only public ones, but today everyone gets the same
  unfiltered list (there's no org visibility field yet to filter on).
- **Hearts tab sort/visibility controls were speced but never built** — see
  [Hearts](#hearts) above. Low-impact since the underlying API only ever
  returns public blueprints regardless of filter, but the missing sort
  control is a real gap against the spec's design.
- **Agents tab visibility filter was speced but never built** — see
  [Agents](#agents-internal-view-only) above.
- **Blueprints tab has no "Most hearted" sort and no pagination** — both
  speced, neither built; harmless today since the account-scoped blueprint
  list is small, but would degrade on an account with many blueprints.

## Verify

```
go test ./internal/account/... ./internal/heartstore/... ./handlers/... -run 'Account|Heart'
cd apps/astro-client && bun x vitest run src/pages/AccountProfile
```

`apps/astro-client/src/components/account-profile/` (`EarlyAdopterBadge`,
`SocialLinksEditor`, `PronounsSelect`) has no dedicated test files; it's
covered only indirectly through `ProfileEditSidebar.test.tsx` and
`ProfileViewSidebar.test.tsx`.
