# Deployment Spec Normalization (Phase 1)

## Summary

Deployment specs were stored as a single JSON blob in `deployments.deployment_spec_json`, making it impossible to query individual fields, detect drift against K8s state, or enforce constraints at the DB level. This change normalizes the spec into proper relational tables while maintaining full backward compatibility.

## Design

The schema is modeled from the EKS perspective — each component in the deployment spec maps to exactly one K8s workload (Deployment, StatefulSet, CronJob, or Job), and the tables mirror the desired K8s state rather than the spec's logical structure. This makes drift detection a simple DB query vs K8s API comparison.

**New tables:**
- `deployment_workloads` — one row per K8s workload (agent, model, knowledge, tool, ingestion, messaging, collector) with resources, GPU, update strategy, healthcheck
- `deployment_services` — K8s Service per workload endpoint (port, protocol)
- `deployment_ingresses` — K8s Ingress per exposed service (hostname, TLS)
- `deployment_volumes` — PVC specs for StatefulSets (size, storage class, access mode)
- `deployment_env_vars` — resolved environment per workload
- `deployment_variables` — top-level user/provider variables with encryption support

**New columns on `deployments`:** `encrypted_data_key` (bytea) and `kms_key_arn` (varchar) for KMS envelope encryption.

**Dual-write:** On deploy, both the JSON blob (backward compat) and normalized rows are written in a single transaction. The old `SaveDeployment()` API is preserved as a wrapper — all existing callers and read paths are untouched.

**Secret encryption:** When `KMS_KEY_ARN` is configured, secret variable values are encrypted via AWS KMS envelope encryption (AES-256-GCM with per-deployment data keys). Without KMS, secrets are stripped as before.

## Migration

No action required. Atlas auto-applies the new tables on startup. Existing deployments retain their JSON blobs. A future backfill script will populate normalized tables for historical deployments.
