# Fix: bound knowledge store credentials never reached the agent

## Summary

An agent bound to an external/PrivateLink knowledge store received the store's `*_HOST` and `*_PORT` env vars but **none of its credentials** (`POSTGRES_USER`/`PASSWORD`/`DB`, etc.). The agent could see where the database was but had no way to authenticate.

## Design

Agent env is assembled by the K8s applier through three independent mechanisms:

- **host/port/url** — `template.go` auto-injects `${knowledge.X.host}` references, resolved into the agent ConfigMap from the bound store info.
- **credentials** — `knowledgeCredEnvVars` emits `secretKeyRef` env vars pointing at a per-store credential Secret created by `ensureKnowledgeCredentialSecrets`.

That Secret was only created for **self-hosted** stores (with auto-generated passwords); bound stores were skipped (`if knowledge.IsBound() { continue }`). With no Secret, `knowledgeCredEnvVars` had nothing to reference and silently emitted no credential env vars — while host/port (a separate path) still flowed. Hence "host+port only."

The fix materialises a bound store's resolved credentials (decrypted from the external store by the deployer and passed in as `boundCredentials`) into the same per-store Secret, keyed by the provider's literal storage keys (`POSTGRES_USER`/…) so `knowledgeCredEnvVars` references them exactly like a self-hosted store. Unlike self-hosted secrets — whose generated password must stay stable — the bound secret is refreshed each deploy to track the current external credentials.

This bug sat in the applier path. A prior change corrected the parallel `deployment.Resolve` model (the `deployment_build_env` record rows), and its tests passed, but the running pod's env comes from the applier — which is now covered by an end-to-end test that runs `ApplyDeploymentSpec` for a bound store and asserts the agent container's `secretKeyRef` env, the cred Secret contents, and the resolved host in the ConfigMap.

## Migration

Agent env is injected at deploy time. Existing agents bound to external stores must be **redeployed** after this ships to pick up the credentials; rechecking the store alone does not re-inject agent env.
