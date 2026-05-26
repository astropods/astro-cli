# Fix CLI token fetch and refresh timing

## Summary

Authenticated CLI commands were failing intermittently with "token expired" or registry `Invalid IdP credential` errors even when a valid refresh token was stored. Two root causes: tokens were fetched once at command start and reused for the entire run (including long builds and polls), and refresh eligibility relied on stored `ExpiresAt` alone — which could drift from the JWT `exp` after org-scoped refreshes or profile upgrades.

This change fixes the highest-impact paths (`ast push`, `ast secrets`, core refresh logic) and establishes a shared pattern for the remaining commands.

## Design

### Refresh when the JWT says so

`shouldRefresh` now treats stored `ExpiresAt` and JWT `exp` as independent signals. If either is within the 5-minute refresh threshold, the CLI refreshes before use. This closes the gap where a bearer was already expired but stored metadata still looked valid.

`ForceRefreshAccessToken` is exported for unconditional refresh (401 recovery). Account helpers expose this as `forceAccountToken(ctx, account)`.

### Fetch at use time, not command start

The old pattern was:

```
cmdAuth() → AccountToken{Token} → pass Token through pipeline / apiCall
```

That token could sit idle through a multi-minute Docker build and then fail at registry `/token` exchange or server registration.

**Push pipeline:** `PushPipelineConfig` no longer carries a pre-fetched token. Each step that needs credentials calls `getAccountToken(ctx, account)` at execution time:

- **Registry push** — `getDockerRegistryAuth` fetches a fresh WorkOS bearer immediately before `ImagePush`, so Docker exchanges it for a registry-scoped token while it is still valid.
- **Register / visibility** — same fresh fetch per HTTP call; registration retries once on 401 via `forceAccountToken`.

WorkOS access tokens are short-lived (~5–10 min). Registry push tokens are ~1 hour and minted at push start — the WorkOS bearer must be fresh at that moment, not at build start.

### `apiCallForAccount` — fetch + 401 retry

New helper in `cmd/utils.go`:

```
getAccountToken(account) → apiCall → on 401 → forceAccountToken → retry once
```

`ast secrets` commands are migrated to this helper. Secrets no longer thread `at.Token` from an initial `cmdAuth`; every API call gets a fresh token and automatic recovery from a mid-flight 401.

Account resolution stays in `cmdAuth` / `getCurrentAccountToken` (account name + verbose). Credentials are resolved per request via `getAccountToken` / `apiCallForAccount`.

### Org refresh token rotation

`GetOrgScopedAccessToken` always persists a rotated refresh token returned by WorkOS. Org-scoped access tokens remain ephemeral (not saved to profile), but the stored refresh token stays current — a follow-on to the April fix that only handled rotation when the token changed.

## Follow-up: consistent auth across all CLI commands

This PR intentionally scopes migration to push and secrets. The following still use `cmdAuth()` once and pass `at.Token` to `apiCall` / `apiStream` with no 401 retry:

| Area | Commands / flows | Risk |
|------|-------------------|------|
| `agent.go` | list, get, stop, start, restart, logs, batch status | Logs use `apiStream` with a cached token; no retry |
| `agent_deploy.go` | deploy, redeploy (`runDeployWithRequest`) | Template POST and deploy POST share one token |
| `blueprint.go` | list, get, create, install, archive, visibility | Install is multi-step with one token |
| `knowledge.go` | create, link, list, get, delete, credentials, logs | **Highest risk:** `pollKnowledgeReady` / `pollKnowledgePrivateLink` poll up to 15 minutes with the token captured at command start |

Recommended completion work:

1. **Bulk migrate** `apiCall(..., at.Token, ...)` → `apiCallForAccount(..., at.Account, ...)` in `agent.go`, `agent_deploy.go`, `blueprint.go`, and `knowledge.go`.
2. **Add `apiStreamForAccount`** — same fetch + 401-retry-on-open semantics as `apiCallForAccount`, used by `agent logs` and `knowledge logs`.
3. **Long-running loops** — poll helpers should call `getAccountToken` (or `apiCallForAccount`) on each iteration, not accept a token string from the caller.
4. **Multi-step flows** — deploy/redeploy and blueprint install should fetch fresh credentials before each HTTP call, especially the final deploy POST after user interaction or server-side validation.
5. **Reduce regression surface** — consider narrowing `AccountToken` to account-only (drop cached `Token`) or documenting that `Token` is informational and must not be passed to `apiCall`.
6. **`ASTRO_ACCESS_TOKEN`** — env override bypasses refresh by design (CI); document that long-running local commands should not rely on it.

Target invariant for all authenticated commands:

> Resolve account once; fetch credentials at each API boundary; retry once on 401.

## Migration

No action required. Existing stored credentials benefit from JWT-aware refresh on next use. Users who were re-running `ast login` after push or secrets failures should see those commands succeed without re-auth.
