## Summary

Platform-provisioned (“managed”) knowledge stores let tenants spin up databases inside Astro infrastructure via `POST .../knowledge`. That path is turned off by default so accounts can only register stores they already operate via the existing connect flow (`POST .../knowledge/connect`).

## Design

- **Configuration**: `DeploymentConfig.KnowledgeAllowManagedCreate`, loaded from **`KNOWLEDGE_ALLOW_MANAGED_CREATE`** (enabled only when the value is the literal `true`, consistent with other boolean env flags).
- **Handler**: `CreateKnowledgeStore` rejects with **403** after normal request validation when the flag is false, with an error message that points callers at the connect endpoint. Listing, detail, delete, logs, metrics, credentials, and **ConnectKnowledgeStore** behavior are unchanged.
- **OpenAPI**: Document **403** on managed create so generated clients expect the failure mode.

## Migration

Operators who still need managed provisioning in an environment must set **`KNOWLEDGE_ALLOW_MANAGED_CREATE=true`** on astro-server. Everyone else needs no client or spec changes beyond handling **403** on managed create if they were calling it.
