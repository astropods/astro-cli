# Supabase Knowledge Store — Architecture

This document explains how the Supabase knowledge-store integration works end to
end: the "Supabase is really Postgres" design decision, the WorkOS Pipes OAuth
brokering, the server endpoints, and the client UI.

## 1. The Core Idea

A "Supabase knowledge store" is **not** a new store type. Under the hood it is an
ordinary **external PostgreSQL** store pointed at a Supabase project:

```
host:     db.<project-id>.supabase.co
port:     5432
database: postgres
username: postgres
password: <supplied by user>
```

`supabase` exists only as a **client-side picker entry**. Selecting it triggers
an OAuth import flow that auto-fills the connection fields; when the store is
actually created it is submitted with `provider: "postgres"`. The database
`provider` column is never `"supabase"`, so every downstream code path
(deploy, bind, health-check) is untouched.

```mermaid
flowchart LR
    A["User picks<br/>'Supabase' in picker"] --> B["OAuth import flow<br/>(auto-fill host/port/db/user)"]
    B --> C["User enters<br/>DB password"]
    C --> D["Store created with<br/>provider = 'postgres'"]
    D --> E["Downstream logic sees<br/>a normal Postgres store"]

    style A fill:#3ECF8E,color:#000
    style D fill:#cde,color:#000
```

**Trade-off:** after creation a Supabase store is indistinguishable from any
other Postgres store — there is no per-store "Supabase" badge. The OAuth
connection is per-**account/user**, not per-store.

## 2. OAuth is brokered by WorkOS Pipes

The integration does **not** hand-roll OAuth or store tokens. Supabase is
registered in WorkOS Pipes as a **custom OAuth provider** (slug `supabase`) —
the same mechanism GitHub and Slack already use. WorkOS owns the hard parts:

| Concern | Owner |
|---|---|
| OAuth callback hosting | **WorkOS** |
| Token storage (access + refresh) | **WorkOS** |
| Token refresh | **WorkOS** |
| Client credentials (id/secret) | **WorkOS** custom-provider config |
| Serving a fresh access token on demand | **WorkOS** (`GetAccessToken`) |
| Calling `api.supabase.com/v1/projects` | astro-server (with WorkOS-served token) |

The server side is a thin wrapper over `internal/pipes.Client`
(`handlers/supabase.go`), mirroring the `GitHubAccount*` handlers.

```mermaid
flowchart TB
    subgraph client["astro-client (React) — unchanged contract"]
        CF["ConfigureForm.tsx (add-store inline)"]
        CS["ConnectorsSettings.tsx (Settings)"]
        QH["api/queries/supabase.ts (hooks)"]
    end

    subgraph server["astro-server (Go)"]
        H["handlers/supabase.go<br/>(thin wrappers over pipes.Client)"]
        P["internal/pipes.Client"]
        H --> P
    end

    subgraph external["External"]
        WK["WorkOS Pipes<br/>(hosts callback, stores + refreshes tokens)"]
        SB["Supabase Management API<br/>(api.supabase.com/v1/projects)"]
    end

    CF --> QH
    CS --> QH
    QH -->|"/api/v1/accounts/:account/supabase/*"| H
    P -->|"authorize / token / delete connection"| WK
    WK -->|"brokers OAuth"| SB
    H -->|"GET /v1/projects (bearer token from WorkOS)"| SB

    style client fill:#eef,color:#000
    style server fill:#efe,color:#000
    style external fill:#fee,color:#000
```

### File responsibilities

| Layer | File | Responsibility |
|-------|------|----------------|
| Client UI | `pages/knowledge/NewKnowledgeStore/ConfigureForm.tsx` | Inline connect card + project picker in add-store |
| Client UI | `pages/settings/ConnectorsSettings.tsx` | `SupabaseSection` — connect/disconnect from settings |
| Client data | `api/queries/supabase.ts` | `useSupabaseStatus/Connect/Projects/Disconnect` hooks |
| Client data | `api/queries/keys.ts`, `lib/api.ts` | `supabaseKeys` factory + `ApiClient` methods + `SupabaseProject` |
| Client meta | `components/knowledge/knowledge-utils.ts`, `ProviderIcon.tsx` | Provider maps + Supabase logo |
| Server | `handlers/supabase.go` | 6 handlers wrapping `pipes.Client` (connect, status, projects, project health, disconnect, callback) |
| Server | `internal/pipes/client.go` | WorkOS Pipes API (`GetAuthorizationURL`, `GetAccessToken`, `DeleteConnection`) |
| Server | `main.go` | Route registration with the shared `pipesClient` |

There is **no** Supabase token store, KMS code, PKCE code, or Supabase env var —
all removed in favor of WorkOS Pipes.

## 3. The OAuth Flow

```mermaid
sequenceDiagram
    autonumber
    participant U as Browser
    participant S as astro-server
    participant WK as WorkOS Pipes
    participant SB as Supabase

    U->>S: POST /accounts/:account/supabase/connect { redirect_to }
    Note over S: Already connected?<br/>GetAccessToken succeeds → {connected:true}
    S->>WK: GetAuthorizationURL(provider=supabase, user, org, ReturnTo=callback)
    WK-->>S: authorize URL
    S-->>U: { redirect_url }
    U->>WK: follow redirect_url
    WK->>SB: brokered OAuth (WorkOS is the registered redirect_uri)
    SB->>U: user authorizes (picks organization)
    U->>WK: consent
    WK->>U: 302 → ReturnTo (/api/v1/accounts/:account/supabase/callback)
    U->>S: GET .../supabase/callback?redirect_to=… (session cookie rides along)
    S->>WK: GetAccessToken (confirm token landed)
    WK-->>S: access token
    S->>U: 302 → <frontend>/<redirect_to>?supabase_connected=true
```

### Key properties

- **No account in a public callback.** The callback is
  `/api/v1/accounts/:account/supabase/callback` — authenticated and
  account-scoped, behind `ResolveAccount` + `RequireAccountMember`, exactly like
  the GitHub callback. The browser's session cookie rides the redirect, so there
  is no unauthenticated redirect endpoint and no CSRF state to manage (WorkOS
  handles the OAuth `state`).
- **PKCE / client secret** live in the WorkOS custom-provider definition, not in
  our code or config.
- **Open-redirect protection.** `isSafeRedirectPath` rejects anything that isn't
  a same-origin relative path before the browser is bounced to `redirect_to`.

## 4. Token Lifecycle — WorkOS owns it

There is no token freshness logic in astro-server. Every endpoint that needs the
Supabase API asks WorkOS for a token and reacts to the result:

```mermaid
flowchart TD
    Start["handler needs Supabase access"] --> G["pipes.GetAccessToken(supabase, user, org)"]
    G -->|"ErrNotInstalled /<br/>ErrNeedsReauthorization"| NC["not connected<br/>(422 or connected:false)"]
    G -->|"other error"| E["500 / connected:false"]
    G -->|token| USE["call api.supabase.com/v1/projects"]
    USE -->|401| REJ["supabase rejected token →<br/>422 supabase_not_connected"]
    USE -->|ok| OK["200 { projects }"]

    style NC fill:#fdd,color:#000
    style REJ fill:#fdd,color:#000
    style OK fill:#3ECF8E,color:#000
```

- **"Connected" = WorkOS can serve a token** (mirrors GitHub/Slack). An expired
  *access* token still reads as connected because WorkOS refreshes it on demand —
  there is no access-token-expiry check on our side.
- A token WorkOS can't serve → "not connected"; a token Supabase rejects with
  401 → `supabase_not_connected` (prompt reconnect).

## 5. HTTP Endpoints

All routes are account-scoped and authenticated (`ResolveAccount` +
`RequireAccountMember`). The callback is part of the same group — no separate
public route.

| Method | Path | Handler | Pipes call |
|--------|------|---------|-----------|
| POST | `/api/v1/accounts/:account/supabase/connect` | `SupabaseConnect` | `GetAccessToken` (short-circuit) then `GetAuthorizationURL` |
| GET | `/api/v1/accounts/:account/supabase/status` | `SupabaseStatus` | `GetAccessToken` |
| GET | `/api/v1/accounts/:account/supabase/projects` | `SupabaseListProjects` | `GetAccessToken` + `GET /v1/projects` |
| GET | `/api/v1/accounts/:account/supabase/projects/:ref/health` | `SupabaseProjectHealth` | `GetAccessToken` + project health fetch |
| DELETE | `/api/v1/accounts/:account/supabase` | `SupabaseDisconnect` | `DeleteConnection` |
| GET | `/api/v1/accounts/:account/supabase/callback` | `SupabaseCallback` | `GetAccessToken` (confirm) |

**Config:** none beyond `WORKOS_API_KEY` (already used for auth). Supabase client
credentials live in the WorkOS custom-provider definition. `SupabaseHandlerConfig`
only carries `WebhookBaseURL` (for the `ReturnTo` callback) and `FrontendURL`.

## 6. Client UI

The client is **unchanged** by the WorkOS migration — the API contract
(connect/status/projects/disconnect + `SupabaseProject`) is identical.

- **Add-store flow (`ConfigureForm.tsx`)** — the projects query doubles as the
  connection check: a `422 supabase_not_connected` means "run OAuth". Before
  OAuth: a focused connect card. After: provider header + project picker;
  selecting a project auto-fills host/port/db/user and reveals the database
  password field. On submit, `provider` is rewritten to `"postgres"`.
- **Settings → Connectors (`SupabaseSection`)** — connect/disconnect alongside
  GitHub and Slack; optimistic disconnect with rollback behind a confirmation
  dialog. Uses `useSupabaseStatus`, which is now accurate at all times because
  WorkOS manages token freshness.
- `?provider=supabase` on the add-store URL pre-selects the provider so the OAuth
  round-trip returns straight to the Supabase form; `supabase_connected` /
  `supabase_error` params are stripped after read.

## 7. Test Coverage

`handlers/supabase_test.go` reuses the shared WorkOS/HTTP test harness
(`rewriteTransport`, `injectTestSession`, `injectTestAccount`) to cover: status
(connected / not-connected), list-projects (success / token-revoked-by-API →
422), disconnect, callback (success / not-authenticated), connect
(already-connected), and the pure helpers (`isSafeRedirectPath`, `appendParam`).
