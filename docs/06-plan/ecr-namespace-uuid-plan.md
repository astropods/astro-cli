# ECR Namespace: Switch from Account Name to Account UUID

## Goal

Use the account UUID as the ECR namespace instead of the account name slug, so ECR repos are
stable across account renames. Backward compatibility with existing builds is required and free
because `ecr_namespace` is frozen at push time — the deploy code uses it as-is.

## Background

ECR repos are currently named: `{env}-tenant-{accountName}/{imageName}`
Target:                        `{env}-tenant-{accountUUID}/{imageName}`

The CLI push URL (`registry/{accountName}/image`) must NOT change — it is user-facing.
The registry proxy translates the name → UUID internally before building the ECR path.

Existing `agent_versions.ecr_namespace` rows store the account name slug. They will keep
working because the deploy code in `template.go` uses `ecr_namespace` as-is to build the
ECR path. Old builds deploy from `prod-tenant-{name}/...`, new builds from
`prod-tenant-{uuid}/...`. No data migration needed.

---

## Files to change

### 1. `apps/astro-registry/internal/account/membership.go`

Add `GetAccountID(name string) (string, error)` method to `MembershipChecker`.
Queries `SELECT id FROM accounts WHERE name = $1`.

This is a point lookup on an indexed unique column — no caching needed for now.

---

### 2. `apps/astro-registry/handlers/registry_proxy.go`

**In `RegistryProxy` handler** — after namespace access is validated, resolve the UUID:

```
namespace := extractNamespaceFromPath(path)   // "myaccount"
resolvedNS := namespace                        // default: fall back to name
if mc != nil {
    if id, err := mc.GetAccountID(namespace); err == nil {
        resolvedNS = id
    } else {
        log.Warn("could not resolve account UUID, falling back to name", ...)
    }
}
```

The fallback to name preserves existing behavior for any edge case (deleted account,
DB hiccup, tests with no DB).

**`extractRepositoryName(path, env, resolvedNS string) string`**
- Change signature to accept `resolvedNS`
- Use `resolvedNS` instead of `parts[0]` for the tenant prefix

**`addTenantPrefix(path, env, resolvedNS string) string`**
- Same: accept `resolvedNS`, use it instead of `parts[0]`

**`buildTargetURL(registryURL, path, query, env, resolvedNS string) (string, error)`**
- Thread `resolvedNS` through to `addTenantPrefix`

**`rewriteLocationHeader`** — no change.
Location headers are rewritten back to the original account name so client-facing
URLs remain human-readable. `stripTenantPrefix` strips `{env}-tenant-` regardless of
what follows, so it already handles UUID-based ECR paths correctly.

---

### 3. `apps/astro-server/handlers/agents.go`

The register handler at `POST /api/v1/agents/:account/:name/register` currently passes
`accountName` (URL path param) as `ecrNamespace` to `index.Register`.

Change to pass `accountID` (already resolved from `acct.ID` earlier in the handler).

```go
// before
index.Register(accountID, agentName, req.BuildID, req.Registry, accountName, ...)

// after
index.Register(accountID, agentName, req.BuildID, req.Registry, accountID, ...)
```

---

### 4. `apps/astro-server/internal/riverqueue/github_build.go`

Revert the recent change (commit `c000b321`) that switched to `conn.AccountName`:

```go
// ecrImagePath — revert to AccountID
destination = w.ecrImagePath(conn.AccountID, agentName, args.BuildID)

// Register — revert to AccountID as ecrNamespace
w.agentIndex.Register(conn.AccountID, agentName, ..., conn.AccountID, ...)
```

---

## Files intentionally NOT changed

| File | Why |
|------|-----|
| `apps/astro-cli/cmd/push.go` | CLI push URL stays `registry/{accountName}/image` — user-facing |
| `apps/astro-server/internal/deployment/template.go` | Already uses `ecr_namespace` as-is — works for both name and UUID |
| `apps/astro-server/internal/agentindex/index.go` | No change to storage logic |
| `sql/astro-server/schema.sql` | `account_name` on `github_connections` kept — useful display data |

---

## Backward compatibility

| Scenario | Behavior |
|----------|----------|
| Old build (ecr_namespace = name slug) | Deploys from `prod-tenant-{name}/...` — images exist there ✓ |
| New build (ecr_namespace = UUID) | Deploys from `prod-tenant-{uuid}/...` — images pushed there ✓ |
| Registry lookup fails | Falls back to name slug — existing ECR path, same as today ✓ |
| Account rename | Old builds unaffected (frozen ecr_namespace). New builds use UUID → immune to future renames ✓ |

---

## Open questions before implementing

1. **Registry `GetAccountID` caching** — the method is called on every push/pull request
   through the proxy. The accounts table is indexed on `name` (unique), so the query is
   fast. Accept the extra round-trip for now; add caching if it shows up in latency.

2. **Test coverage** — `apps/astro-registry` has tests for `validateNamespaceAccess` and
   `extractRepositoryName`. These need updating to pass `resolvedNS` and to cover the
   fallback path.
