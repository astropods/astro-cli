# Messaging and chat interfaces

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

This doc covers the **integration surface** between astro-server/astro-client
and the messaging system: how a WorkOS user's Slack identity gets linked, how
the web chat widget is built, and how the CLI embeds the same chat experience
locally. It does not document the messaging sidecar's internals.

Three related docs each own a different layer — read this one for the
frontend/CLI/identity glue, and the other two for what they cover:

- [`01-spec/slack-adapter-spec.md`](../01-spec/slack-adapter-spec.md) — the
  Slack Socket Mode adapter inside the messaging sidecar (event handling, AI
  status/prompts APIs, rate limiting, thread hydration). That adapter lives in
  `modules/messaging` (an independently-versioned git submodule, ~210 files);
  this doc does not duplicate its internals.
- [`04-guides/deployment-chat.md`](../04-guides/deployment-chat.md) — the
  backend API contract for deployment chat: REST/SSE endpoints, auth, chat
  persistence (SQLite in the sidecar, not astro-server/RDS), resumable
  streaming, and turn termination. This doc assumes that contract and covers
  what consumes it.

## Three chat-shaped surfaces, one sidecar

Every deployed agent that supports chat runs a **messaging sidecar**
(`modules/messaging`) alongside it. The sidecar hosts multiple **adapters**,
each a thin protocol translator that funnels into the same gRPC
`ProcessConversation` bidirectional stream to the agent:

- `internal/adapter/web` — serves the REST + SSE contract
  `04-guides/deployment-chat.md` documents. This is what astro-client's chat
  widget and the CLI's embedded chat SPA both talk to, through astro-server's
  proxy.
- `internal/adapter/slack` — serves Slack's Socket Mode WebSocket, translating
  Slack events to `pb.Message` and agent responses to Slack API calls (status
  updates, buffered replies, feedback buttons). See the spec doc above for
  detail.

The two adapters are independent interfaces to the *same* agent conversation
plumbing: a user can message an agent from the Astro web UI, the CLI's local
chat, or Slack, and each is a distinct adapter instance, not three copies of
the chat logic. Slack does not go through astro-server's chat proxy at all —
it connects directly to the sidecar's Socket Mode client from Slack's
infrastructure. astro-server's only role in the Slack path is **identity
linking** (below), not message transport.

## Slack identity linking (astro-server)

This is unrelated to the Slack *adapter* above — it is the account-level
"Connect Slack" flow that links a Slack identity to a WorkOS user, so
Insights can attribute Slack-originated agent usage to a real user.

### OAuth flow (`internal/slack`, `handlers/slack.go`)

`internal/slack` (`client.go`) implements a raw Slack OAuth client — deliberately
not via WorkOS Pipes' Slack template, because Pipes' template is bot-token
shaped: its `GetAccessToken` returns an `xoxb-*` token whose `auth.test`
resolves to the *bot* user, not the human installer. To get the linking
human's `slack_user_id`, the flow needs the user token Slack's
`oauth.v2.access` issues in `authed_user.access_token`, and reads
`authed_user.id` directly from that response (no follow-up `auth.test` call).

Handlers in `handlers/slack.go`, mounted under
`/api/v1/accounts/:account/slack/...`:

| Method | Path | Purpose |
|---|---|---|
| POST | `/connect` | Builds the `slack.com/oauth/v2/authorize` URL (`user_scope=users:read,team:read`) and sets a CSRF state cookie. |
| GET | `/callback` | Verifies CSRF state, exchanges the code, best-effort enriches with `team.info` (workspace icon) and `users.info` (display name), and upserts the mapping via `slackidentity.Store`. |
| GET | `` (status) | Lists every active Slack workspace the current WorkOS user has linked. |
| DELETE | `` (disconnect) | Revokes one mapping (`?team_id=`) or all of the user's mappings. |

The callback also calls `UsersList` (paginated, capped at 50 pages) to seed
the workspace directory into `slack_observed_users`, so Insights can render
*unlinked* Slack users (name, avatar, workspace deep link) without asking a
deployed agent to do a per-message Slack profile lookup.

`handlers/slack_display.go` holds pure display-name formatting helpers
(`slackDisplayNameFromProfile`, `slackDisplayNameFromUsername`) — e.g.
detecting a stale lowercase-handle display name and title-casing the
username instead. `handlers/observability_slack_resolve.go` is the read-time
hydrator: given a batch of `user_id`s from an Insights response, it
classifies each as Slack- or Astro-kind (`classifyUserID`) and joins in
display metadata from `slackidentity.Store` or the account's personal
profile table, respectively, batched and deduplicated per call.

### The `slackidentity` package: DB layer + a shared ID matcher

`internal/slackidentity` (`store.go`) is the data-access layer for
`slack_identity_mappings` (OAuth-linked, one row per WorkOS user × Slack
team) and `slack_observed_users` (directory/live-seen data, keyed by
team+Slack-user, with no WorkOS link required). Key operations: `Upsert`
(link), `Revoke`/`RevokeOne` (soft-delete), `Lookup` (resolve a Slack sender
to a WorkOS user for the messaging container's authorization path),
`DirectoryEntriesForSlackUserIDs` (Insights' unscoped fallback join for
legacy Langfuse rows with no `team_id`), and `UpsertObserved`/
`UpsertObservedProfiles` (live and bulk directory writes, the former
deduplicated in-memory per pod for 24h to bound write amplification).

`slackid.go` is a separate, tiny concern: `IsBareSlackUserID` — a single
regex-shaped check (`U` + 8-11 uppercase alphanumerics, i.e. length 9-12)
that recognizes a raw Slack user ID in a Langfuse trace's `userId` field
(every historical Slack trace carries a bare ID like `U0ALENLUWBG` before any
identity join happens). It is the discriminator the observability handler
uses to decide which `userId`s need a Slack-directory join, and that the
directory-backfill River worker uses to filter Langfuse's distinct-user-ID
list before upserting.

**This matcher is duplicated in the frontend, not shared — confirmed drift
risk.** `apps/astro-client/src/components/activity/insights-user-identity.ts`
defines its own `SLACK_BARE_RE = /^U[A-Z0-9]{8,11}$/` and `isSlackUserId()`,
independently reimplementing the same shape check as
`slackidentity.IsBareSlackUserID`. The two are equivalent today (both accept
length 9-12, `U` + uppercase-alphanumeric), but there is no shared source:
`slackid.go`'s own doc comment says "Mirrors astro-client's SLACK_BARE_RE —
keep all three in sync" (the third being an in-code reference elsewhere),
which is a manual-sync instruction, not an enforced contract. A change to one
regex silently desyncs from the other with no test or lint to catch it.

A second, smaller instance of the same pattern: the frontend's
`slackDisplayNameFromProfile` / `isStaleSlackHandleDisplayName` /
`slackDisplayNameFromUsername` in `insights-user-identity.ts` reimplement
(not import) the same fallback logic as
`apps/astro-server/handlers/slack_display.go`'s
`slackDisplayNameFromProfile` / `isStaleSlackHandleDisplayName` /
`slackDisplayNameFromUsername` — same names, same algorithm, ported by hand
across the Go/TypeScript boundary and kept in sync only by the frontend
file's comment ("Keep the display-name fallback logic aligned with
apps/astro-server/handlers/slack_display.go").

Both duplications exist because Go and TypeScript can't share source; there
is no proto/codegen boundary here to unify them. Treat any change to the
bare-ID shape or the display-name fallback rules as a two-repo-side change
until/unless this is consolidated (e.g. by moving the regex/logic behind an
API response field instead of re-deriving it client-side).

## Web chat widget (astro-client)

### Page and workspace shell

`src/pages/chat/Chat.tsx` is the chat page. It is **cross-account**: it lists
every chat-eligible agent across every account the signed-in user belongs to
(`useChatAgents`), with no left-rail account switcher — switching agents is
the thread-header dropdown. It resolves `deploymentId` from the URL param,
falls back to redirecting to the first eligible deployment, and renders
`ChatEmptyState` (loading / error / no-agents / agents-not-chattable variants)
until an agent is selected, then renders `ChatWorkspace`.

`ChatWorkspace` (`src/components/chat/ChatWorkspace.tsx`) owns:
- the active `conversation` URL search param (not React state, so the
  browser back button and deep links work),
- one-time auto-selection of the most recent conversation per agent
  (`useChatSessions`), which stops re-triggering once done so a deliberate
  "New chat" isn't bounced back,
- the thread header (agent picker, conversation history dropdown, rename/
  delete), the `StorageCapacityBanner`, the `ChatThread` itself, and a
  `SidePanel`-hosted `ChatInspectorPanel` (agent config/overview/settings
  tabs) toggled independently of the conversation.

`ChatThread` (`src/components/chat/ChatThread.tsx`) reads deployment
status/runtime (`useDeploymentStatus`, `useDeploymentRuntime`) to derive
composer state (`deriveChatComposerState`), then wraps
`DeploymentChatThreadView` in `DeploymentChatRuntimeProvider`.

### `DeploymentChatRuntimeProvider`: bridging to assistant-ui

The chat UI is built on the third-party
[`@assistant-ui/react`](https://github.com/assistant-ui/assistant-ui)
library. `DeploymentChatRuntimeProvider`
(`src/components/chat/DeploymentChatRuntimeProvider.tsx`) is the adapter
between Astro's own chat state (`useDeploymentChat`) and assistant-ui's
`useExternalStoreRuntime`:

- converts Astro's message list to assistant-ui's `ThreadMessageLike[]` via
  `chatMessagesToThreadMessages` (`src/lib/messaging/chat-message-adapter.ts`),
- derives `isRunning` from whether the last message is a completed assistant
  turn (so the composer doesn't flip to "Send" mid-turn and drop typed input),
- registers a dictation adapter (`src/lib/chat/dictation.ts`, browser Web
  Speech API) and, when the agent's capabilities report `files: true` (gated
  on sidecar reachability via `useDeploymentChatReadiness` so the
  agent/config proxy is never fired at a stopped deployment), a file
  attachment adapter (`src/lib/messaging/deployment-attachment-adapter.ts`),
- publishes a separate `DeploymentChatStreamingContext`
  (`src/components/chat/deployment-chat-streaming-context.tsx`) carrying
  viewport-only state (streaming message id, history-loading, pending
  interaction, error) that `DeploymentChatThreadView` reads directly —
  kept separate from the assistant-ui runtime because that state doesn't fit
  assistant-ui's message-list shape.

### `useDeploymentChat`: the state machine

`src/hooks/use-deployment-chat.ts` is the core of the widget — a long hook
that owns:

- **TanStack Query as the message source of truth.** The cache entry for a
  conversation (`chatKeys.conversation(deploymentId, conversationId)`) is
  patched in place by inbound SSE chunks during a live turn
  (`patchConversationAssistantChunk`) and invalidated on `finish`/`error` so
  the next fetch's persisted, server-assigned ids replace temporary
  streaming ids.
- **One `EventSource` per in-flight conversation, keyed by conversation id**
  (`streamsRef`), not by "the conversation on screen." A turn's stream stays
  open across a conversation switch and is closed only when the turn ends or
  the hook unmounts — so navigating away mid-reply doesn't kill the agent's
  answer; a background finish or error is tracked in `backgroundErrorsRef`
  and surfaced when the user returns to that conversation.
- **Two independent timeout backstops**, layered on top of the sidecar's own
  server-authoritative turn termination (`04-guides/deployment-chat.md`
  §Turn termination): a 90s **liveness watchdog** (`armWatchdog`, reset by
  any inbound SSE event including heartbeats — a dead pipe, not a slow
  turn) and a 15-minute **content-stall cap** (`armStallTimer`, reset only by
  content chunks — catches a sidecar that keeps heartbeating but never
  produces output or a terminal event). Both are pure client-side transport
  backstops; the server's own idle watchdog and disconnect handler are the
  primary mechanism.
- **Blocking interactions** (`pendingInteraction` /
  `clearPendingInteraction`): a live interaction arriving over SSE
  (`onInteraction`) takes precedence over the persisted `pending_interactions`
  queue on the conversation response (which lags mid-turn); resolved
  interaction ids are locally suppressed until the next server refetch drops
  them from the queue.
- **Send-time optimistic append + lazy conversation creation**: sending on a
  blank chat calls `createMessagingConversation` first, seeds the cache with
  the user's message, then `sendMessagingMessage`; on an existing
  conversation it appends optimistically and lets the send call fail loudly
  (rolling back the optimistic row) rather than silently. A `message_limit_reached`
  API error code (not a bare 409) is treated as terminal and shown via toast,
  not as a retryable stream error — see the sidecar contract note in
  `04-guides/deployment-chat.md`.

### Blocking-interaction form renderer (`interaction/`)

`src/components/chat/interaction/` renders a JSON-Schema-driven form for a
blocking interaction (e.g. a tool-permission gate or a structured
data-collection prompt) sent by the agent:

- `types.ts` defines the renderer's field model (`FieldDescriptor`,
  `FieldKind` — text/textarea/code/number/boolean/select/multiselect) and
  re-exports the shared `Interaction`/`JsonSchema` domain types from
  `src/lib/chat/interaction.ts` (the data layer that also parses/validates
  interactions coming off the wire).
- `schema.ts` flattens a JSON Schema's `properties` into ordered
  `FieldDescriptor`s (`describeFields`), infers a field's kind from its
  schema shape (`enum` → select, array-of-enum → multiselect, `x-ui.widget`
  → code/textarea), seeds initial form values from a prefilled `value`
  (`initialFormValue`), and computes which required fields are still empty
  (`missingRequired`).
- `InteractionForm.tsx` renders the form (via `InteractionField.tsx` per
  field) plus the action row, gated on the interaction's declared
  `actions: InteractionAction[]` (`submit` / `decline` / `cancel` /
  `respond`). A `tool_permission` intent renders as a permission gate
  (humanized tool name, Approve/Deny) instead of a generic form heading. A
  `respond` action switches to free-text mode ("Write your own reply") for
  when the structured form doesn't fit what the user wants to say.

### Conversation list

`src/hooks/use-chat-sessions.ts` wraps `useDeploymentChatConversations`,
gated on `useDeploymentChatReadiness` (so the list isn't fetched against a
stopped/unreachable deployment, which would 5xx and trip the per-route
alert). It maps the server response to the UI's `ChatSession` shape and
exposes `recordFirstMessage`, called after a send to invalidate the list —
the sidecar derives the conversation's title and recency server-side
(`EnsureForSend`), so the client only needs to refetch, never write, that
metadata.

## CLI's embedded chat SPA (`modules/astro-cli`)

`ast dev` / `ast project` serve the **same chat React code** locally, so the
CLI's local chat experience and the deployed web chat can't visually drift.

### Build: `vite.chat-embed.config.ts`

`apps/astro-client/vite.chat-embed.config.ts` is a separate, SSR-free Vite
build of the chat experience, entered via `src/chat-embed/main.tsx` and
`chat-embed.html`. It deliberately omits the `@react-router/dev` plugin
(plain SPA, not the framework/SSR build `react-router build` produces) and
outputs to `chat-embed-dist/` (built via `bun run build:chat-embed`, which
also renames `chat-embed.html` to `index.html`). That output is copied into
`modules/astro-cli/internal/chatui/webdist/` at CLI release-build time (named
`webdist`, not `dist`, because the repo-root `.gitignore` ignores every `dist`
directory).

`src/chat-embed/main.tsx` mounts `ChatPage` (the exact same component the
deployed app uses) under a minimal router, with:
- no WorkOS auth — an already-authenticated `AuthContext` value is supplied
  directly, with one synthetic personal account (`LOCAL_ACCOUNT = "local"`,
  matching `chatui.LocalAccount` on the Go side — the comment in both files
  flags this as a pairing to keep in sync by hand),
- no SSR theme/server context beyond a hardcoded `DEFAULT_THEME`,
- routes limited to `/chat` and `/chat/:deploymentId`.

### Serving: `internal/chatui`

`modules/astro-cli/internal/chatui/server.go` is the Go HTTP server that:
- embeds the prebuilt SPA via `//go:embed all:webdist` (`embed.go`); when
  only the tracked `.gitkeep` placeholder is present (a plain `go build`
  without the release asset-copy step), it serves a clear 503 instead of a
  confusing 404, so local development against an unbuilt asset dir fails
  loudly,
- synthesizes the same deployment-summary/list/status/runtime read endpoints
  astro-server exposes in production, but for a single hardcoded local
  deployment (`LocalDeploymentID = "local"`) — `types.go`'s response shapes
  are commented as intentionally matching the frontend's TypeScript response
  interfaces so the embedded client deserializes them unchanged,
- reverse-proxies `/api/v1/deployments/:id/{chat,messaging,files}/*` to the
  local messaging sidecar's native `/api/*` / `/api/chat/*` / `/api/files*`
  routes — the same path rewrite astro-server's chat/messaging proxy performs
  in production, so the embedded React runs unmodified whether the agent is
  local or deployed.

`modules/astro-cli/cmd/chatui.go` is the process-management layer around that
server: a hidden `chatui-serve` cobra command spawns as a **detached,
session-leader** process (survives the launching CLI dying in background
mode; killed via `--exit-with-parent` in foreground mode), tracked by a pid
file (`.chatui.pid`) under the project's `.ast` dir. Because the chat UI
binds a **fixed, shared port** (`127.0.0.1:3100`) across all local projects,
`startChatUI` first stops this project's own recorded worker, then reclaims
the port from any other `chatui-serve` process still holding it (a
force-quit leak, or another project's worker) via `lsof` + SIGTERM/SIGKILL,
before spawning. A post-spawn health probe (`HealthPath =
/__chatui/health`, which echoes the responding process's pid) distinguishes
"our worker came up" from "the port answers, but it's some other worker" —
so the CLI never advertises `localhost:3100` as ready when it's actually
serving a stale process.

`modules/astro-cli` is a separate, independently-versioned git repository
(a private submodule per the root `CLAUDE.md`); only its "Initial public
release" commit is visible from this checkout, so no meaningful git-log churn
signal exists here — treat any git-blame-based read of `internal/chatui` or
`cmd/chatui.go` history as unavailable rather than absent.

## Known gaps and things to watch

- **The Slack bare-ID regex and display-name fallback logic are duplicated,
  not shared**, between `apps/astro-server/internal/slackidentity/slackid.go`
  + `apps/astro-server/handlers/slack_display.go` and
  `apps/astro-client/src/components/activity/insights-user-identity.ts`. See
  "The `slackidentity` package" above. No test or lint enforces the two stay
  in sync; a change to one and not the other would silently misclassify or
  mis-render Slack users in Insights.
- The Slack OAuth callback's directory sync (`UsersList` → paginated, capped
  at 50 pages of 200 users = 10,000 users) is best-effort: a failure logs a
  warning and leaves the previously-synced directory in place rather than
  failing the link. A very large workspace beyond the page cap gets a
  partial directory silently (logged, not surfaced to the user).
- `internal/slack`'s `OAuthClient` methods (`TeamInfo`, `UserInfo`,
  `UsersList`) each build their own raw HTTP request rather than sharing a
  single authenticated-request helper; the pattern (build request, execute,
  read body, check `ok`) is repeated four times in `client.go` with minor
  variation.
- The CLI's `LocalAccount`/`LocalDeploymentID` constants
  (`internal/chatui/server.go`) and the frontend's `LOCAL_ACCOUNT`
  (`src/chat-embed/main.tsx`) are a second instance of the cross-language
  "keep two hardcoded strings in sync by comment" pattern seen in the Slack
  identity matchers above — lower risk here (a mismatch would just break the
  local dev chat shell, not misattribute production data), but the same
  shape of drift risk.

## Verify

- Slack identity/OAuth (Go): `go test ./internal/slack/... ./internal/slackidentity/...`
  (from `apps/astro-server`); `go test ./handlers/... -run 'Slack|UserDetails'`
  for the callback/display/hydration handlers.
- Chat widget (astro-client): `cd apps/astro-client && bun x vitest run src/components/chat src/hooks/use-deployment-chat.test.tsx src/hooks/use-deployment-chat-readiness.test.tsx src/lib/chat src/lib/messaging src/pages/chat src/api/queries/chat.test.tsx`
- CLI embedded chat SPA (Go): `cd modules/astro-cli && go test ./internal/chatui/...`
  — one test file (`server_test.go`), exercises the files-proxy path rewrite;
  the health/pid/port-reclaim logic in `cmd/chatui.go` has no automated test
  and can only be verified by running `ast dev` locally.
