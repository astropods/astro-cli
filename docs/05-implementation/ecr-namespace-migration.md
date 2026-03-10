# ECR Namespace Migration & Agent Transfers

## Problem

ECR repositories use account names in their paths (`{env}-tenant-{account_name}/{image}`). This couples image storage to a mutable display identifier, making account renames and agent transfers require image copying. We want to decouple the physical image location from account ownership so transfers become a DB update with no image operations.

## Design

Add `ecr_namespace` to `agent_versions`. Each version records the account name that was active when it was pushed — the physical ECR location of its images. The registry proxy and CLI are unchanged. The indirection only matters at deploy time, where `resolveImage` reads the version's `ecr_namespace` instead of parsing the account name from the stored image path.

- **Existing versions**: backfill `ecr_namespace` from the owning account's current name.
- **New versions**: set `ecr_namespace` to the pushing account's name at registration time.
- **Transferred agents**: `account_id` changes on agent + version rows; `ecr_namespace` stays. Images don't move.
- **Re-push after transfer**: new versions get the new account's name as `ecr_namespace`. Old versions keep theirs.

---

## Phase 1: Schema

```sql
ALTER TABLE agent_versions ADD COLUMN ecr_namespace text;

UPDATE agent_versions av
   SET ecr_namespace = acc.name
  FROM agents a
  JOIN accounts acc ON acc.id = a.account_id
 WHERE a.account_id = av.account_id AND a.name = av.name;

ALTER TABLE agent_versions ALTER COLUMN ecr_namespace SET NOT NULL;
```

No changes to `accounts` or `agents` tables.

---

## Phase 2: Agent Index

**`apps/astro-server/internal/agentindex/index.go`**

Add `ECRNamespace string` to `AgentVersion`:

```go
type AgentVersion struct {
    BuildID            string
    ECRNamespace       string
    Spec               map[string]any
    Readme             string
    ValidationWarnings []map[string]any
    PublishedAt        time.Time
    UpdatedAt          time.Time
}
```

### Register

The register handler passes the account name. `Register()` writes it as `ecr_namespace`:

```go
_, err = tx.Exec(`
    INSERT INTO agent_versions
        (account_id, name, build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    ON CONFLICT (account_id, name, build_id)
    DO UPDATE SET spec_json = $5, readme = $6, validation_warnings = $7, updated_at = $9
`, accountID, name, buildID, ecrNamespace, string(specJSON), readme, validationWarnings, now, now)
```

### Read paths

All version queries (`Get`, `GetVersion`, `GetLatestVersion`, `ListForAccount`, `ListPublicAgents`) add `ecr_namespace` to SELECT and scan it into `AgentVersion.ECRNamespace`.

### Transfer

Moves ownership. `ecr_namespace` on each version is untouched:

```go
func (idx *Index) Transfer(sourceAccountID, targetAccountID, agentName string) error {
    tx, err := idx.db.Begin()
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    now := time.Now()

    _, err = tx.Exec(`
        UPDATE agents SET account_id = $1, updated_at = $2
        WHERE account_id = $3 AND name = $4
    `, targetAccountID, now, sourceAccountID, agentName)
    if err != nil {
        return fmt.Errorf("failed to transfer agent: %w", err)
    }

    _, err = tx.Exec(`
        UPDATE agent_versions SET account_id = $1, updated_at = $2
        WHERE account_id = $3 AND name = $4
    `, targetAccountID, now, sourceAccountID, agentName)
    if err != nil {
        return fmt.Errorf("failed to transfer versions: %w", err)
    }

    return tx.Commit()
}
```

---

## Phase 3: Deploy-Time Resolution

### template.go

**`apps/astro-server/internal/deployment/template.go`**

Add `ECRNamespace` to `TemplateInput`:

```go
type TemplateInput struct {
    Spec              *spec.AstroSpec
    Account           string // account name (display, used in DeploymentSource)
    ECRNamespace      string // version's ecr_namespace — where images physically live
    BuildID           string
    RegistryURL       string
    ProxyRegistryHost string
    Environment       string
}
```

Change `resolveImage` to use `input.ECRNamespace` instead of `parts[0]`:

```go
// 1. Tenant image → ECR tenant repo
if input.ProxyRegistryHost != "" && input.RegistryURL != "" &&
   strings.HasPrefix(image, input.ProxyRegistryHost+"/") {
    pathWithTag := strings.TrimPrefix(image, input.ProxyRegistryHost+"/")
    parts := strings.SplitN(pathWithTag, "/", 2)
    if len(parts) >= 2 {
        return fmt.Sprintf("%s/%s-tenant-%s/%s",
            stripScheme(input.RegistryURL),
            input.Environment,
            input.ECRNamespace, // was: parts[0]
            parts[1])
    }
    return image
}
```

### image_resolver.go

**`apps/astro-server/internal/k8s/image_resolver.go`**

Add `ecrNamespace` field:

```go
type ImageResolver struct {
    proxyRegistryHost string
    ecrRegistryURL    string
    environment       string
    ecrNamespace      string
}

func NewImageResolver(proxyRegistryHost, ecrRegistryURL, environment, ecrNamespace string) *ImageResolver {
    return &ImageResolver{
        proxyRegistryHost: proxyRegistryHost,
        ecrRegistryURL:    ecrRegistryURL,
        environment:       environment,
        ecrNamespace:      ecrNamespace,
    }
}
```

In `ResolveImage`:

```go
tenantNamespace := r.environment + "-tenant-" + r.ecrNamespace // was: namespace
```

### deploy.go

**`apps/astro-server/handlers/deploy.go`**

Look up the version and pass its `ecr_namespace`:

```go
version, _ := agentIndex.GetVersion(sourceAcct.ID, agentName, buildID)

template, _ := deployment.GenerateDeploymentTemplate(deployment.TemplateInput{
    Spec:              astroSpec,
    Account:           sourceAcct.Name,
    ECRNamespace:      version.ECRNamespace,
    BuildID:           buildID,
    RegistryURL:       cfg.RegistryURL,
    ProxyRegistryHost: cfg.ProxyRegistryHost,
    Environment:       cfg.Environment,
})
```

---

## Phase 4: Transfer API

**New: `apps/astro-server/handlers/transfer.go`**

```
POST /api/v1/agents/{account}/{agent}/transfer
{ "target_account": "bob" }
```

1. **Auth**: caller must be member of both source and target accounts.
2. **Collision check**: agent name must not exist in target account. Reject with 409.
3. **Transfer**: `agentIndex.Transfer(sourceAcct.ID, targetAcct.ID, agentName)`.
4. **Namespace ownership**: update `source_account` for active deployments of this agent.
5. **Active deployments**: keep running. Each version's `ecr_namespace` still resolves to the correct ECR repos. No redeployment required.

### Post-transfer behavior

Agent `myagent` transferred from `alice` to `bob`:

- Version `a1b2c3d4` has `ecr_namespace = "alice"`. At deploy time → `{env}-tenant-alice/myagent:a1b2c3d4`. Correct.
- Bob pushes new version `e5f6g7h8`. CLI pushes to `registry.host/bob/myagent:e5f6g7h8`. Registry proxy routes to `{env}-tenant-bob/myagent:e5f6g7h8`. Register handler sets `ecr_namespace = "bob"` on the new version.
- Deploy old version → resolves to alice's ECR. Deploy new version → resolves to bob's ECR. Both correct.

---

## Unchanged Components

- **Registry proxy** (`apps/astro-registry/handlers/registry_proxy.go`): no changes. Continues mapping `{account_name}` → `{env}-tenant-{account_name}`.
- **CLI** (`apps/astro-cli/cmd/push.go`): no changes. Pushes to `registry.host/{account_name}/agent:build`.
- **Account store** (`apps/astro-server/internal/account/store.go`): no changes.
- **Schema for `accounts` and `agents`**: no changes.

---

## Change Summary

| File | Change | Breaking? |
|------|--------|-----------|
| `schema.sql` | Add `ecr_namespace` to `agent_versions` | No (additive, backfilled) |
| `agentindex/index.go` | Read/write `ecr_namespace` on versions; add `Transfer()` | No |
| `template.go` | Add `ECRNamespace` to `TemplateInput`; use in `resolveImage` | No |
| `image_resolver.go` | Accept `ecrNamespace` param | No |
| `deploy.go` | Pass version `ECRNamespace` into template input | No |
| New: `handlers/transfer.go` | Transfer API endpoint | N/A (new) |

## Rollout Order

1. **Schema migration** — add column, backfill from owning account name.
2. **Server deploy** — agent index reads `ecr_namespace`; all existing versions resolve to same paths as before.
3. **Deploy handler** — passes `version.ECRNamespace` to template generation.
4. **Transfer API** — new endpoint. Works immediately for all agents, no preconditions.
5. **Client UI / CLI** — add transfer command/UI as needed.

## Account Rename

After rename (`alice` → `alice2`), existing versions keep `ecr_namespace = "alice"` and resolve correctly — those ECR repos still exist. New pushes under `alice2` create new ECR repos and new versions get `ecr_namespace = "alice2"`. Optional cleanup: background job to copy old manifests to new-name repos and update `ecr_namespace` on affected version rows.
