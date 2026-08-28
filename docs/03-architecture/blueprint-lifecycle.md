# Blueprint lifecycle: registry, build, push, versioning

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-27

This doc covers the backend registry that every agent build lands in:
`agentindex`, the two ways a build gets registered (CLI push and GitHub
build), the caches that sit around it, and how a registered version turns
into a signed deployment spec. For the GitHub build pipeline itself (webhook,
BuildKit job, ECR push), see
[`github-connection.md`](github-connection.md) — this doc only covers the
one call that pipeline makes into the registry. For what happens after a
deploy call is made, see
[`deployment-state-machine.md`](deployment-state-machine.md). For the
browse/detail/configure/deploy frontend, see
[`blueprint-deploy-flow.md`](blueprint-deploy-flow.md).

---

## Terminology

The codebase uses four words that overlap in casual conversation but map to
precise, distinct things in `agentindex`:

| Term | What it actually is |
|---|---|
| **Agent** | The Go struct and DB row (`agents` table) for one `(account_id, name)` pair. Doc comment: "Agent represents an agent with all its versions." This is the durable identity: its name, registry string, visibility, avatar, archive state. |
| **Blueprint** | The product-facing name for the same `agents` row. There is no separate `blueprints` table or Go struct — `Blueprint` only appears in list/query function and type names (`BlueprintListOptions`, `ListForAccount`, `ListPublicAgents`, `CreateBlueprintRequest`, `UserBlueprint`). "Agent" and "blueprint" refer to the same row; use whichever the surrounding code/API already uses. |
| **Version** | One row in `agent_versions`, identified by `build_id`. Doc comment: "AgentVersion represents a specific published build of an agent." A version has no semantic number: it's an opaque `build_id` string plus a `spec_json`/`readme`/`agent_card_json` payload and a `published_at` timestamp. |
| **Build** | A synonym for version at the API/DB level, not a separate concept. `build_id` is the version's primary identifier; error strings say `"build not found: %s"`; `DeleteVersion` operates on a `build_id`. There is no separate "build" record distinct from the `agent_versions` row it produces. |

One agent has zero or more versions. "Latest" is never stored — it's always
computed as `MAX(published_at)` or `ORDER BY published_at DESC LIMIT 1`. There
is no `is_latest` flag and no semantic versioning; a version is only ever
addressed by its `build_id` or by "whichever one is newest right now."

---

## Data model

Two tables, in `sql/astro-server/schema.sql`:

```sql
CREATE TABLE agents (
    account_id       uuid NOT NULL,
    name             text NOT NULL,
    registry         text NOT NULL,
    visibility       varchar(10) NOT NULL DEFAULT 'private',
    archived_at      timestamp,
    name_reserved    bool NOT NULL DEFAULT false,
    avatar_colors    jsonb,
    avatar_updated_at timestamptz,
    created_at       timestamp NOT NULL,
    updated_at       timestamp NOT NULL,
    PRIMARY KEY (account_id, name)
);

CREATE TABLE agent_versions (
    account_id          uuid NOT NULL,
    name                text NOT NULL,
    build_id            text NOT NULL,
    ecr_namespace       text NOT NULL DEFAULT '',
    spec_json           text NOT NULL,
    readme              text NOT NULL DEFAULT '',
    agent_card_json     jsonb NOT NULL DEFAULT '{}',
    validation_warnings text NOT NULL DEFAULT '',
    published_at        timestamp NOT NULL,
    updated_at          timestamp NOT NULL,
    PRIMARY KEY (account_id, name, build_id),
    FOREIGN KEY (account_id, name) REFERENCES agents(account_id, name)
        ON UPDATE CASCADE ON DELETE CASCADE
);
```

Notes on individual columns:

- **`agents.name_reserved`** — set `true` the first time an agent goes
  public, and never cleared, even if the agent later goes private again or
  is archived. This stops a name from being recycled by someone else once it
  was ever public. `MarkNameReserved` also sets it independently, best-effort,
  right after a deployment is created for the agent — so a name in active use
  by a deployment is reserved even if it was never made public.
- **`agent_versions.ecr_namespace`** — the account UUID the build's image was
  pushed under. Both registration paths (CLI push and GitHub build) pass the
  account's own UUID here, matching the ECR tenant-path convention
  (`{env}-tenant-{account_uuid}/...`) documented in
  [`github-connection.md`](github-connection.md#ecr-image-path) and in
  [`registry-token-auth.md`](registry-token-auth.md). This is deliberately
  independent of `agents.registry`, which is a display string the CLI
  constructs from the human-readable account name
  (`{registryHost}/{accountName}`) — `ecr_namespace` is what's actually used
  to resolve images; `registry` is informational.
- **`agent_versions.spec_json`** — the full `astropods.yml`, minus the `name`
  key (`Register` deletes it before storing; the canonical name lives only on
  the `agents` row).
- Related tables joined or touched by the registry: `agent_hearts` (FK
  `ON UPDATE CASCADE` on `(account_id, agent_name)`, so a `Transfer` re-keys
  it automatically), `deployments` (has its own `source_account_id`/`agent_name`
  columns, **not** cascaded — `Transfer` updates these manually), and
  `audit_logs` (used to attribute publishers by `action = 'agent.register'`).
  `github_builds`/`github_connections` are **not** re-keyed by anything: no
  FK ties either table's `account_id` to `agents`, and `Transfer` doesn't
  touch them. After a transfer they keep pointing at the source account,
  which silently breaks `accountBlueprintLatestCommitJoin`'s
  `gb.account_id = a.account_id` join for the transferred agent, losing its
  commit metadata. Real gap, not yet fixed; see doc-drift-log.md.

---

## `agentindex.Index` — the registry

Package: `apps/astro-server/internal/agentindex`. Backed directly by
`*sql.DB`; no ORM.

### Register

```go
func (idx *Index) Register(accountID, name, buildID, registry, ecrNamespace string,
    spec map[string]any, readme string, agentCardJSON string, validationWarnings string) error
```

This is the single write path both CLI push and GitHub build funnel into (see
[Two ways to register a build](#two-ways-to-register-a-build) below). One
transaction:

1. Deletes `spec["name"]` before marshaling — the name lives only on `agents`.
2. `INSERT INTO agents ... ON CONFLICT (account_id, name) DO UPDATE SET
   registry, updated_at, archived_at = NULL` — creates the agent shell if it
   doesn't exist, and **un-archives it unconditionally** if it was archived.
   There's no separate "restore" step; any successful push on an archived
   name just brings it back.
3. `INSERT INTO agent_versions ... ON CONFLICT (account_id, name, build_id)
   DO UPDATE` — upserts the version row. Re-registering the same `build_id`
   (e.g. a retried push) overwrites in place rather than erroring.
4. Commits, then calls `invalidateBlueprintLists(accountID)` outside the
   transaction, best-effort (error discarded).

`Register` does **not** set visibility — that's a separate call. The HTTP
handler (`RegisterAgent`) calls `index.SetVisibility` right after `Register`
when the request includes a `visibility` field, on every register call, not
just the first one (the `RegisterAgentRequest.Visibility` field comment says
"only applied on first registration," but the code doesn't actually gate on
that — it applies whatever visibility the caller sends every time).

### Other mutations

- **`Create(accountID, name) error`** — creates an empty agent shell with no
  version (`registry = ''`). Used by "connect a GitHub repo before pushing."
  If an archived agent of the same name exists, un-archives it and **deletes
  all its old versions first** (comment: "build IDs are only created by
  `ast push`, not the UI flow" — starts as a clean draft). Returns
  `ErrAlreadyExists` if a live (non-archived) agent already has that name.
- **`Archive(accountID, name) error`** — soft delete: sets `archived_at`.
  Rejects (returns an error) if the agent is already archived. Versions are
  preserved, just hidden from list queries (`WHERE archived_at IS NULL`
  everywhere they're listed).
- **`DeleteVersion(accountID, name, buildID) error`** — hard-deletes one
  `agent_versions` row. If it was the last version, hard-deletes the `agents`
  row too.
- **`SetVisibility(accountID, name, visibility string) error`** — only
  accepts `"public"` or `"private"`. Side effect:
  `name_reserved = (name_reserved OR visibility == "public")` — going public
  is a one-way trapdoor for name reservation, independent of whether the
  agent is later set back to private.
- **`Transfer(sourceAccountID, targetAccountID, agentName) error`** —
  re-keys `agents.account_id`. `agent_versions` and `agent_hearts` move
  automatically via `ON UPDATE CASCADE`; `deployments.source_account_id` is
  updated in a separate explicit statement because that FK isn't part of the
  cascade. `ecr_namespace` on existing versions is **left unchanged on
  purpose** — the images already pushed under the old account UUID keep
  resolving; a transfer doesn't reshuffle ECR. Only new versions of the
  transferred agent get the new account's `ecr_namespace`.
- **`SetAvatarColors`, `TouchAvatarUpdatedAt`, `MarkNameReserved`** — small
  targeted updates, each busting the blueprint list cache.

Every one of these mutations ends by calling
`idx.invalidateBlueprintLists(accountID)` (and, for `Transfer`, both source
and target account IDs).

### Reads

- **`Get(accountID, name) (*Agent, error)`** — the `agents` row only.
  `Versions` comes back empty: every caller but the detail endpoint reads
  visibility, name, or avatar colors, and loading build payloads for them
  shipped whole specs and readmes that were then discarded.
- **`GetWithVersions(accountID, name) (*Agent, error)`** — the agent plus all
  its versions, newest first, each with its full `spec_json`, `readme`, and
  `agent_card_json`. Only `GET /agents/:account/:name` needs this, because it
  renders build history.
- **`GetVersion(accountID, name, buildID)`**, **`GetLatestVersion(accountID, name)`**.
- **`ValidateLineage(accountID, name, buildID) error`** — a
  `SELECT 1 ... LIMIT 1` existence probe (index-only against
  `agent_versions_pkey`), returning only whether the row exists. It reads no
  payload columns and unmarshals nothing, because it runs on the deploy path
  and during template prefill. Exists so `*Index` implicitly satisfies
  `deploymentstore.LineageValidator` without that package importing
  `agentindex`. **Deliberately does not filter on `agents.archived_at`** — a
  version published before its agent was archived still validates. This is
  called out explicitly in the source because it's a real, considered policy
  choice: an existing deployment must remain redeployable after its source
  agent is archived, so tightening this to exclude archived agents would
  break redeploys of "legacy" deployments and needs its own migration/policy
  decision, not an incidental fix.
- **`List()`** — every non-archived agent, no visibility filter. Internal/
  admin use only; nothing in the read path relies on this for
  visibility-sensitive display.
- **`AgentNames(accountID)`** — just names, for an account.
- **`BatchLatestBuildIDs`/`BatchLatestBuildInfo`** — one SQL round trip via
  `unnest(...)` returning the latest `build_id` (and, for the `Info`
  variant, `visibility`) for many `(account_id, name)` pairs at once.
  `BatchLatestBuildIDs` picks the latest with a `ROW_NUMBER()` window
  function; `BatchLatestBuildInfo` instead does it per row with a
  `JOIN LATERAL ... ORDER BY published_at DESC LIMIT 1`, same result,
  different query shape. This is what powers the "update
  available" pill on a deployment: a deployment's lineage
  (`source_account_id`, `agent_name`) is checked in bulk against the
  registry's current latest build, without an N+1 query per deployment.

### List and search

`ListForAccount`, `ListPublicAgents`, and
`ListVisibleBlueprintsForUserPage` (cross-account, cursor-paginated "my
blueprints" feed) all share filter/order building
(`agentindex/blueprint_list.go`, `blueprint_list_query.go`):

- `BlueprintListOptions{Query, Tag, Visibility, Sort, Limit, Offset}`.
- Text search (`Query`) matches `agents.name` (`ILIKE`) or the **latest**
  version's `agent_card_json->>'description'` or its `tags` array — older
  versions aren't searched.
- `Sort: "newest"` orders by the latest version's `published_at`; the
  default orders by name.
- `escapeLike` neutralizes `%`, `_`, and `\` in user-supplied search text
  before it goes into a `LIKE`/`ILIKE` pattern.

`ListVisibleBlueprintsForUserPage` additionally does keyset pagination
(a `(published_at, name, account_id)` cursor, not offset-based) across every
account a user belongs to, and enriches results via
`BatchUserBlueprintMetadata` — one query joining `agent_hearts`,
`agent_message_counts`, `deployments`, and an `audit_logs`-derived publisher
list: every distinct actor who has ever called `agent.register` for that
agent, one entry each, ordered earliest-first by their first registration
(`MIN(created_at)`), each resolved to their personal-account display name.

---

## Visibility

Stored as a plain string column, no DB enum, checked in two places:

- **At write time**, `SetVisibility` rejects anything other than
  `"public"`/`"private"`, and reserves the name permanently on the
  public transition (see above).
- **At read time**, in the handlers, not in `agentindex` itself:
  `GetAgent` 404s a private agent for a non-member; `ListAccountAgents`
  forces `Visibility: "public"` into the list options when the caller isn't
  an account member; `ListPublicAgents`'s `WHERE` clause hardcodes
  `visibility = 'public'` unconditionally (it's only ever used for the public
  catalog).

There's no separate "unlisted" or "org-only" tier — visibility is a strict
public/private binary.

---

## Caches

### `blueprintcache` — invalidation, not storage

`apps/astro-server/internal/blueprintcache`. This package stores no list
payloads. It's a **generation token** per account:

```go
const SafetyTTL = time.Hour
var generations = listcache.NewGenerations("blueprint:generation:", SafetyTTL, 4096)

func Invalidate(ctx, cache, accountID string) error
func Generations(ctx, cache, accountIDs []string) []string
```

Every `agentindex` mutation (`Register`, `Create`, `Archive`, `DeleteVersion`,
`SetVisibility`, `MarkNameReserved`, `SetAvatarColors`,
`TouchAvatarUpdatedAt`, `Transfer`) bumps the calling account's generation.
The actual response cache is the generic `listcache` package (shared by other
list endpoints, not blueprint-specific): it mixes the current generation
value into its cache key, so a stale cached page for an account is never
served after that account's data changes — no explicit cache deletion, no
scanning for keys to invalidate. `handlers.ListUserBlueprints` is the
consumer, wiring `blueprintcache.Generations` in as
`serveUserResourceList`'s `generations` function (cache key prefix
`usr:list:blueprints:v2:`).

### `imagecache` — unrelated to blueprint metadata

`apps/astro-server/internal/imagecache` does **not** cache anything about
agents or versions. It force-evicts one specific ECR pull-through cache
entry: the messaging sidecar image
(`dockerhub/astropods/messaging:latest`), because ECR only re-checks Docker
Hub for a pull-through-cached tag once every ~24 hours. `Refresher.RefreshMessaging`
calls `ecr.BatchDeleteImage` on that one repo:tag so the next agent pull picks
up a fresh Docker Hub image immediately instead of waiting out the window.
It's invoked from an admin action, not from the register/push path — include
it here only because the task surface asked about it; it isn't part of the
blueprint registry proper.

---

## Two ways to register a build

Both paths end at the exact same `agentindex.Index.Register` call, but reach
it differently and validate slightly differently. There is no shared
"register a build" function above `agentindex` itself — each pipeline
assembles the call independently.

### CLI push (`ast push`)

Traced through `modules/astro-cli/cmd/pipeline.go` (`PushPipeline`):

```
ParseSpec → CollectComponents → ResolveVisibility → Build → Push →
TransformSpec → StripSecrets → LoadReadme → UploadReadmeAssets → Register
```

- **Build ID**: `generateBuildID()` — 8 hex characters from `crypto/rand`,
  generated client-side once per push, reused as the image tag for every
  component and as the `agent_versions.build_id`.
- **Build**: local. `PushPipeline.Build()` runs BuildKit against the
  developer's own Docker daemon, one image per component from
  `spec.CollectComponents`. The server never builds anything for a CLI push.
- **Push**: `pushToRegistry()` pushes each component to
  `{registryHost}/{accountName}/{imageName}:{buildID}` — `registryHost` is
  the **astro-registry proxy**, not ECR directly. Auth is a WorkOS bearer
  token traded for a short-lived registry-scoped JWT (see
  [`registry-token-auth.md`](registry-token-auth.md) for that exchange);
  astro-registry's proxy then rewrites `{accountName}/...` to the real ECR
  path `{env}-tenant-{accountID}/...` server-side. The CLI never sees an ECR
  host or an account UUID.
- **Register call**: `POST /api/v1/agents/:account/:name/register` (handler
  `RegisterAgent`, `apps/astro-server/handlers/agents.go`), body:
  ```json
  {
    "build_id": "...", "registry": "{registryHost}/{accountName}",
    "spec_content": "<transformed, secret-stripped YAML>",
    "readme": "<AGENT.md>", "visibility": "public|private",
    "readme_assets": { "relative/path.png": "https://cdn/..." }
  }
  ```
  The CLI's `TransformSpec` step already stripped secret defaults and
  rewritten `build:` blocks to `image:` references before sending. The
  handler independently re-validates: rejects any secret input that still
  has a default (`spec.SecretDefaultViolations` — a defense against an old
  or misbehaving CLI, since this is a public HTTP endpoint that can't trust
  the caller), runs `deployment.NewValidatorWithOptions(...).ValidateSpec`
  (structural errors reject the whole push with 400; deploy-time-only issues
  like variable/schedule fields become `validation_warnings` stored on the
  version and returned in the response), enforces a minimum CLI version via
  `X-Cli-Version`, and rejects org-scoped (`@org/name`) agent names.
- After `index.Register`, the handler also: busts downstream deploy caches
  for every deployment whose lineage points at this agent
  (`deploycache.InvalidateForLineage` — this is what makes the "update
  available" banner show up immediately), applies `SetVisibility` if
  requested, writes an `AgentRegister` audit event, and generates a
  placeholder avatar if the agent doesn't have one yet.

### GitHub-triggered build

Traced through `apps/astro-server/internal/githubbuild/pipeline.go`
(`GitHubBuildPipeline`) — full webhook/BuildKit/K8s-job detail is in
[`github-connection.md`](github-connection.md); only the registration end is
covered here.

```
FetchSpec → ValidateSpec → ... → RunBuildJobs → FetchReadme →
ProcessReadmeImages → TransformSpec → StripSecrets → Register
```

- **Build ID**: an 8-hex ID assigned when the build is enqueued (same shape
  as the CLI's, so both paths produce build IDs that look identical).
- **Build**: server-side, in a Kubernetes BuildKit job, pushing straight to
  ECR (see `github-connection.md`).
- **Register call**: `p.cfg.AgentIndex.Register(...)` called **directly,
  in-process** — the GitHub build worker runs inside astro-server and holds
  a live `*agentindex.Index`, so there's no HTTP hop and no gin handler for
  this path. It passes `p.cfg.AccountID` for both `accountID` and
  `ecrNamespace`, matching the CLI path's UUID-keyed `ecr_namespace`.
- **Validation**: `ValidateSpec()` runs the *same* validator
  (`deployment.NewValidatorWithOptions(...).ValidateSpec`) with the same
  field-skip rules as the HTTP handler, and turns a structural failure into
  a `PermanentError` that fails the build outright (surfaced as a
  `build.failed` notification, not a 400 response, since there's no HTTP
  caller). `StripSecrets()` strips secret defaults the same way the CLI does
  client-side — but there's no equivalent of `SecretDefaultViolations`
  here, because this pipeline is the only thing that produces the spec it
  registers; unlike the public HTTP endpoint, it doesn't need to defend
  against a caller who skipped stripping.

**Real divergence worth knowing about:** the GitHub pipeline computes the
same non-fatal validation messages (variable/schedule fields) the HTTP path
turns into `validation_warnings`, but discards them — it calls `Register`
with a literal `"[]"` for `validationWarnings` instead of the computed list.
A GitHub-built version's `agent_versions.validation_warnings` is always
empty, even when the same spec pushed via the CLI would have stored
warnings. This means the CLI-push response and UI can surface "this
variable/schedule needs attention at deploy time" for a build, while the
identical spec built from a GitHub push silently drops that signal. It's a
real gap, not a design choice documented anywhere — worth fixing by passing
the computed warnings through, or filing to the doc-drift log if it needs
follow-up.

Both paths converge on `deploycache.InvalidateForLineage` after
registration, so the "update available" signal fires identically regardless
of which path produced the build.

### Quota gating (push only)

`quota.DBChecker.WrapRegister` wraps the `/register` route (CLI push and any
other caller of that HTTP endpoint — GitHub builds bypass this too, since
they call `Register` in-process). It checks `ResourceAgentBuilds` on every
push, and additionally checks `ResourceBlueprints` (the blueprint-count cap)
only when the push's `(account, name)` doesn't already exist as a
non-archived agent — so an account at its blueprint-count limit can still
keep pushing new builds of agents it already has; the cap only blocks
*creating* a new one.

---

## From a registered version to a deploy

`agentindex` versions feed the deploy flow at one seam:
`handlers/deploy.go`'s `generateTemplate` resolves an `*agentindex.AgentVersion`
(by explicit `build_id` or via `GetLatestVersion`) and turns its
`spec_json` into an `AstroDeploymentSpec` (the deployment template). That
template, once finalized, is signed — this is `specsign`, a completely
separate mechanism from anything in `agentindex`:

- **What `specsign` signs**: the deployment template
  (`deployment.AstroDeploymentSpec`) returned by the deployment-template
  endpoint, HMAC-SHA256 over a canonical JSON encoding, with the
  caller-supplied target fields (`Target.Account`, `Target.DisplayName`,
  `Target.DeploymentID`, `Target.ClusterID`) zeroed out before hashing
  because the client sets those *after* receiving the template.
- **Why**: so the deploy endpoint can trust a returned template without
  re-deriving or re-validating it field by field — `specsign.Verify` just
  confirms the spec the client is submitting to `/deploy` is byte-identical
  (apart from those four target fields) to what `/deployment-template`
  generated. It's a tamper-integrity check on the deploy request, not an
  authorization or identity check, and not a spec/artifact signature the way
  a container image signature would be.
- **Where**: signed in `respondDeploymentTemplate` (only when the template
  request has `finalize: true`); verified in `DeployAgent` against the
  `X-Template-Signature` header. `/deploy/validate` skips this check
  entirely because no deploy actually happens there.
- **How it differs from `agentindex.Register`**: `Register` persists the
  agent's own declared build (its `astropods.yml`, as pushed) into the
  registry — permanent, versioned, keyed by `build_id`. `specsign` operates
  one step later, on the ephemeral *deployment* spec the server derives from
  a registered version for one specific deploy call — it's never stored,
  never versioned, and has nothing to do with which builds exist or who can
  push them. Push/register authorization is ordinary `agents:write`
  permission middleware, unrelated to `specsign`.

See [`deployment-state-machine.md`](deployment-state-machine.md) for what
happens once a signed spec reaches `/deploy`.

---

## `blueprint-jellybean` — social-card rendering, not registry logic

`packages/blueprint-jellybean` has real logic (not just static assets): it
builds a 1200×628 SVG "badge" card for a blueprint (`card.ts`,
`buildBlueprintBadgeSvg`) and rasterizes it to PNG via `@resvg/resvg-js`
(`index.ts`, bundled with its own TTF fonts under `fonts/` for consistent
server-side rendering). It's consumed by exactly one place,
`apps/astro-client/src/pages/BadgeAgent.tsx`, which serves the
OpenGraph/social-preview image for a blueprint's public page, using the
`avatar_colors` and agent-card description that `agentindex` already stores.
It isn't part of the registration or deploy path and astro-server never
imports it — it only reads data the registry already produced.

---

## Handlers touching registration, versions, or visibility

All in `apps/astro-server/handlers/`:

| Handler | Route | Calls |
|---|---|---|
| `RegisterAgent` | `POST /agents/:account/:name/register` | `index.Register`, `index.SetVisibility` |
| `CreateBlueprint` | `POST /agents/:account` | `index.Create`, `index.SetVisibility` |
| `ArchiveAgent` | `POST /agents/:account/:name/archive` | `index.Archive` (plus a best-effort background goroutine to unlink any connected GitHub repo/webhook) |
| `SetAgentVisibility` | `PUT /agents/:account/:name/visibility` | `index.SetVisibility` |
| `TransferAgent` (`transfer.go`) | `POST /agents/:account/:name/transfer` | `index.Get` (existence/collision checks), `index.Transfer` |
| `ListAgents`, `ListAccountAgents`, `GetAgent` | `GET /agents`, `GET /agents/:account`, `GET /agents/:account/:name` | `index.ListPublicAgents`, `index.ListForAccount`, `index.Get` |
| `ListUserBlueprints` (`user_blueprints.go`) | `GET /me/blueprints` | `index.ListVisibleBlueprintsForUserPage`, `index.BatchUserBlueprintMetadata` |
| `UploadBlueprintAvatar`, `ResetBlueprintAvatar` (`avatar.go`) | `POST`/`DELETE /agents/:account/:name/avatar` | `index.TouchAvatarUpdatedAt`, `index.SetAvatarColors` |

`agents.go`'s `buildVersionResponse`/`buildLegacyAgentCard` translate a
stored `AgentVersion` into the API response shape, synthesizing an agent
card from legacy `meta.description`/`meta.tags` spec fields for versions
pushed before the agent-card feature existed.

---

## Known sharp edges

- **GitHub builds drop validation warnings** — see
  [above](#two-ways-to-register-a-build); this looks unintentional, not a
  documented design choice.
- **`ValidateLineage` ignores archive state by design** — a version stays
  valid for redeploy after its agent is archived. This is intentional and
  explained in the source, but easy to mistake for a bug if you're only
  looking at `Archive`'s doc comment ("hidden from list queries") without
  also reading `ValidateLineage`.
- **Visibility "only applied on first registration"** is what the request
  struct's comment claims, but the code applies whatever visibility is sent
  on every `Register` call, not just the first. Trust the code, not that
  comment.
