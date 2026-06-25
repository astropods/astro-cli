# Fix avatar caching: fingerprinted URLs + long-lived cache

## Summary

Agent, account, and deployment avatars were effectively never cached: every
view re-validated or re-fetched the image, so they never loaded instantly and
added a network round trip per render. The cause was a deliberate-but-blunt
tradeoff. Avatar URLs are *stable* — the same path forever
(`/avatars/agents/{account}/{name}.jpg`) — so to let an updated avatar appear
quickly, the S3 objects were given a 60-second `max-age`. CloudFront honors the
origin's `max-age` (its `default_ttl` only applies when the origin sends no
`Cache-Control`), so both the browser and the edge expired the image every
minute and revalidated on the next view.

That's the classic stable-URL bind: you can have a stable URL, a long cache, or
fast updates — pick two. This change picks long cache + fast updates by making
the URL change when (and only when) the image changes.

## Design

Standard cache-busting via **fingerprinted URLs**: every avatar carries a
`?v=<token>` that changes only when the underlying image changes, so the URL is
safe to cache for a long time.

- **Version source.** A new nullable `avatar_updated_at` column on `accounts`,
  `agents`, and `deployments` records when each entity's avatar image last
  changed. It is stamped on every avatar mutation — user upload, preset
  assignment, server-generated identity, the periodic identity backfill, the
  deploy-time blueprint→deployment copy, deletes, and renames. It is
  intentionally separate from the entity's `updated_at` so unrelated edits don't
  bust the avatar cache. `NULL` means "unknown" and simply emits no token, so
  existing rows keep working and self-heal on their next avatar write.

- **Token in the URL.** The server appends `?v=<unix(avatar_updated_at)>` when
  it builds an avatar URL, and emits a versioned `avatar_url` for blueprint,
  deployment, **and** account identities — the blueprint/deployment payloads, the
  account/search/member responses, and the `/auth/me` account list all carry it
  (members and the auth list source the token from batched account loads, no
  N+1). The token is the DB-persisted `now()` (returned via `RETURNING`), so the
  immediate upload response and later reads agree on one cache key.

- **Client consumes the server URL.** Components render the server's versioned
  `avatar_url` rather than rebuilding the bare key client-side — otherwise the
  long cache makes them *more* stale. The leaf avatar components resolve
  `local upload override ?? server url ?? handle-derived fallback`, so a
  just-uploaded image shows instantly while the versioned server URL wins on
  every other load. Deployment, blueprint, profile, members, grants, org
  switcher, header, and account-scoped pages all thread the server URL through;
  deployment avatars resolve through a dedicated `DeploymentAvatar` (keyed on the
  deployment, not the blueprint). The few surfaces with only a bare handle in
  scope (breadcrumb, blueprint-author chips, Insights chips, and the
  not-yet-created blueprint in the creation flow) keep the handle-derived
  fallback and self-heal within the cache window.

- **Cache lifetime.** Avatar objects move from `max-age=60` to
  `public, max-age=86400, stale-while-revalidate=604800` (1 day fresh, 7 day
  background revalidation). `Cache-Control` is S3 *object* metadata shared by
  every URL that points at the object, and not every surface is fingerprinted
  (account avatars and Insights chips are built client-side from
  `handle`/`account`+`name`). A finite TTL — rather than `immutable` — keeps
  those unversioned surfaces correct: they load instantly and self-heal within a
  day, while the fingerprinted hot paths (agent/deployment) update instantly
  because their URL changes.

- **Edge cache key (infra).** CloudFront previously ignored query strings
  (`query_string = false`), which would let a new `?v` collide with the old
  cached object at the edge. The `assets` distributions now include `v` in the
  cache key. This ships as a separate `astro-infra` change and must be applied
  together with this one.

## Migration

**Required — apply the schema migration before deploying the new server image.**
This adds `avatar_updated_at` to `accounts`, `agents`, and `deployments`, and the
new server's hot-path reads (account/agent/deployment lookups, personal-profile
batch) `SELECT` it unconditionally. If the image rolls out before the columns
exist, those reads fail with `column "avatar_updated_at" does not exist` — a
core read-path outage. The columns are nullable with no default, so the
`ADD COLUMN` is online/non-blocking; no backfill is needed (NULL renders
unversioned and self-heals on the next avatar write). Apply via the usual Atlas
schema apply (it diffs `sql/astro-server/schema.sql`).

No action required for end users; existing avatars keep working (they render
unversioned until their next write, then pick up a token).

Two more operational notes for rollout:

- The `astro-infra` cache-key change (CloudFront `assets` distributions key on
  `v`) must be applied alongside this release. Without it, fingerprinted URLs
  would still be served stale from the edge for up to the cache lifetime.
- The new `Cache-Control` is written when an object is (re)uploaded; objects
  already in S3 keep their old header until rewritten. To apply the new caching
  to the existing avatar corpus immediately, refresh object metadata in place
  over the `avatars/` prefix (S3 copy with `MetadataDirective=REPLACE`).
  Otherwise it takes effect naturally as avatars are next written.
