# Deployment list reads stop shipping every spec

## Summary

Three multi-row deployment reads selected the full-row column list, so every
row carried `deployment_spec_json` (the whole stored spec), `error_details`, and
the envelope key material. None of their callers read any of it. The deployments
list, the observability account summaries, the Insights rollups, and account
deletion all want metadata: status, namespace, display name, build, avatar
colors.

The cost is heap and TOAST reads plus bytes on the wire. For an account with 375
live deployments the read went from about 4 ms to about 1.1 ms and stopped
shipping 904 KB per request (warm cache, loopback; reproduce with
`EXPLAIN (ANALYZE, BUFFERS)` and `\timing` on both column lists). The Insights
rollup runs this per account, so the saving multiplies.

## Design

`DeploymentMeta` is the projection for multi-row reads: every `deployments`
column except `deployment_spec_json`, `error_details`, `encrypted_data_key`, and
`kms_key_arn`. `GetVisibleDeploymentsByAccount`,
`GetVisibleDeploymentsByAccountAndBuilds`, and `GetDeploymentsByIDsForAccount`
return it. `Deployment` is unchanged and still serves single-row reads, which
genuinely need the spec.

A distinct type rather than a narrower column list on `Deployment`: a struct
with four silently-zeroed fields reads as valid Go, so `dep.DeploymentSpecJSON`
would compile and return `""` at every call site. The compiler now rejects that.
`Deployment.Meta()` projects a full row for the two helpers shared between
single-row and list paths.

**Lineage resolution keeps working without the spec.** `validatedLineagePublisher`
parsed `spec.source.account` as the fallback for legacy rows whose
`source_account_id` is NULL. That one short string now comes from SQL, read only
for the rows that need it:

```sql
CASE WHEN source_account_id IS NULL
     THEN deployment_spec_json::jsonb #>> '{source,account}' END
```

`CASE` is short-circuit, so a row with `source_account_id` set never touches the
spec column. Guarding matters: measured on 375 rows, an unguarded extraction
costs 1.6 ms against 0.66 ms guarded, because the TOAST read dominates once the
transfer is gone. Extracting in SQL follows `GetMessagingWebConfigured`, which
already casts this column (`internal/deploymentstore/normalized.go`).

The publisher *display name* also has a spec fallback, used when the publishing
account has been deleted. The list caller discards the name (`pubID, _ :=`), and
the single-deployment caller still holds the full spec, so that path is
unchanged.

`EnqueueUndeploy` now takes a deployment id and cluster id instead of a
`*Deployment`. It only ever used those two values, and passing them explicitly
lets the account-deletion path hand it a list row.

## Migration

None. No schema change and no API change: the deployments list, observability
summaries, and Insights responses carry the same fields as before.
