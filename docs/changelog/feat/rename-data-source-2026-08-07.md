# Rename data sources

## Summary

A data source's name could only be set at creation. A typo or a team rename meant revoking the key and reconfiguring every machine that used it — a disruptive fix for a display-only problem. Sources can now be renamed in place from the row menu in Settings → Data Sources.

## Design

Rename is a distinct sub-resource, `PATCH /api/v1/accounts/:account/otel-keys/:tokenID/name`, mirroring the existing `/exclusions` endpoint. Keeping it separate from the credential is the point: the update touches only the `name` column, so the token hash, prefix, and exclusion list are untouched and machines already sending telemetry keep working. As with exclusions, the plaintext key is never re-revealed — the rename dialog shows only the name.

The store update is account-scoped and restricted to non-revoked keys (`WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL`), matching `Revoke` and `UpdateExclusions`, so one account cannot rename another's key and a revoked key cannot be resurrected by editing it.

Create and rename now share a `normalizeTokenName` validator that trims and caps length at 200 characters. Previously create only trimmed and rejected empty names; the column is unbounded `text`, so nothing stopped a name too long to render in the sources table. Sharing the validator also prevents the asymmetry where a rename could set a name that create would have rejected.

On the client, `useRenameOtelIngestKey` follows the existing mutation convention and invalidates the account's ingest-key query on success. The dialog prefills the current name and disables Save until the name actually changes, so a no-op rename never reaches the server.

## Migration

No migration is required. The endpoint is additive and no schema change was needed.
