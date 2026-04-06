# ECR Tenant Namespace: Account Name → Account ID

## Summary

ECR repositories were namespaced by account **name** (e.g. `prod-tenant-saswatds/myapp`). Account names are mutable — a rename would silently break all image pulls for any deployed agent. This change makes the ECR tenant namespace use the account's immutable **UUID** instead, so repository paths are stable regardless of account renames.

The CLI is unchanged: it continues to send images tagged with the account name. The translation to UUID happens inside the platform.

## Design

### astro-registry: resolve UUID before touching ECR

The registry proxy previously built ECR paths by splicing the account name directly from the URL path:

```
/saswatds/myapp/manifests/latest  →  prod-tenant-saswatds/myapp
```

Now `MembershipChecker` exposes `IsMemberWithID`, a single query that checks membership **and** returns the account UUID in one round trip:

```sql
SELECT a.id FROM account_members am
JOIN accounts a ON a.id = am.account_id
WHERE a.name = $1 AND am.user_id = $2 AND a.deleted_at IS NULL
```

The resolved UUID is threaded through `extractRepositoryName`, `addTenantPrefix`, and `buildTargetURL`, which now produce UUID-namespaced ECR paths:

```
/saswatds/myapp/manifests/latest  →  prod-tenant-01kgg.../myapp
```

Location headers rewritten back to the client still use the original account name, so Docker clients see no change.

### astro-server: store UUID as ECRNamespace at registration

`handlers/agents.go` previously passed `accountName` as the `ecrNamespace` argument to `agentindex.Register`. The account struct is already in scope at that point; the fix is a one-field change to pass `acct.ID` instead.

Image resolution in `deployment/template.go` (`resolveImage`) and `k8s/image_resolver.go` (`ImageResolver`) use `ECRNamespace` verbatim — no changes needed there.

### Deployment order

astro-registry must be deployed first. New ECR repos are created under UUID paths from that point. astro-server deployed second begins storing UUIDs as `ECRNamespace`. If the order is reversed, new registrations would store UUIDs while the registry still creates name-based repos.

## Migration

No data migration required. Existing `agent_versions` rows retain their `ecr_namespace` values (account names), and the ECR repos under those names still exist. The resolution logic uses `ECRNamespace` verbatim, so:

- **Old builds** (`ecr_namespace = "saswatds"`) → resolve to `prod-tenant-saswatds/image` — repo exists, pulls work
- **New builds** (`ecr_namespace = "01kgg..."`) → resolve to `prod-tenant-01kgg.../image` — new repo, new pushes land there

Both coexist without conflict. A future optional migration can backfill UUIDs and copy images if full consolidation is needed.
