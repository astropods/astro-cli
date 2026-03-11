# Redeploy Support — Backend

## Summary

Three changes to support proper redeploy:
1. New template endpoint pre-fills ALL values (including KMS-decrypted secrets) from an existing deployment
2. Deploy endpoint accepts `target.deployment_id` to update an existing deployment in-place (stable ID)
3. Remove `target.namespace` from spec — server-owned, never client-set

## Design

### Spec: update `DeploymentTarget`

Add `DeploymentID`, remove `Namespace`:
```go
type DeploymentTarget struct {
    Runtime      string `json:"runtime" yaml:"runtime"`
    Account      string `json:"account,omitempty" yaml:"account,omitempty"`
    DisplayName  string `json:"display_name,omitempty" yaml:"display_name,omitempty"`
    DeploymentID string `json:"deployment_id,omitempty" yaml:"deployment_id,omitempty"`
}
```

Namespace flows through `deployContext.k8sNS` → `ApplierConfig.Namespace` (already does). Removing from spec eliminates the `<generated-on-deploy>` placeholder, editable field entry, and resolver validation.

### New store methods

**`GetDeploymentVariables(deploymentID)`** — reads `deployment_variables`. Returns name, value (ciphertext for secrets), secret, optional, targets, nonce.

**`UpdateDeploymentFull(params, txFn)`** — updates existing active deployment in-place: build_id, spec JSON, KMS fields, deployed_at. Cascade-deletes + re-inserts normalized data in same tx.

### New endpoint: pre-filled template

**Route:** `GET /api/v1/agents/:account/:name/deployment-template/:deploymentID`

1. Generate template via shared helper (extracted from existing handler)
2. Look up deployment by ID — get record + variables
3. Verify user membership
4. Decrypt secrets via `envelope.NewDecryptor` (if KMS configured)
5. Merge: `deployment_id`, `display_name`, `account`, all variable values, `interfaces.adapters`

### Deploy endpoint: in-place update

When `target.deployment_id` is set:
1. Look up existing deployment by ID — verify active + user membership
2. Reuse namespace and deployment ID (skip display_name heuristic)
3. After K8s apply, call `UpdateDeploymentFull` instead of `SaveDeploymentFull`

## Key files

| File | Change |
|---|---|
| `packages/astro-spec/deployment_spec.go` | Add `DeploymentID`, remove `Namespace` |
| `apps/astro-server/internal/deploymentstore/normalized.go` | `GetDeploymentVariables` |
| `apps/astro-server/internal/deploymentstore/store.go` | `UpdateDeploymentFull` |
| `apps/astro-server/internal/deployment/template.go` | Remove `target.namespace` from editable |
| `apps/astro-server/internal/deployment/resolver.go` | Remove namespace validation |
| `apps/astro-server/internal/k8s/spec_applier.go` | Remove spec namespace override |
| `apps/astro-server/handlers/deploy.go` | Template helper, new handler, update path |
| `apps/astro-server/main.go` | Register new route |

## Verification

1. `GET .../deployment-template` → unchanged (no namespace field)
2. `GET .../deployment-template/<id>` → all values pre-filled
3. `POST /deploy` without deployment_id → new record (unchanged)
4. `POST /deploy` with deployment_id → in-place update, stable ID + namespace
5. `go test ./...` passes
