# Fix: redeploys break knowledge-credentials Secret

## Summary

Every redeploy of an agent that uses self-hosted knowledge stores
(postgres, redis) left the new build's pod stuck in
`CreateContainerConfigError`, because the Apply pipeline was deleting the
very Secret the new build's `envFrom` referenced. The deployment row sat
in `provisioning` (UI: "Deploying") indefinitely.

## Design

Knowledge credential Secrets (`<agent>-knowledge-<name>-creds`) are the
only resource the applier creates *once* and reuses across redeploys —
so the postgres password the running StatefulSet was initialized with
stays stable. On the first deploy they're created with the current
build's `app.kubernetes.io/version` label.

On subsequent deploys, `ensureKnowledgeCredentialSecrets` saw
`AlreadyExists`, read the existing data back, and moved on without
updating the Secret's metadata — so its `version` label kept pointing at
the build that first wrote it. Then `cleanupStaleBuildResources` (the
final phase of `ApplyDeploymentSpec`) listed all managed Secrets for the
agent and deleted those whose `version` label didn't match the current
build. The knowledge-creds Secret matched that filter and was deleted on
every redeploy. The new build's pod then failed to mount it.

The fix patches the existing Secret's labels to the current build on
reuse. The Secret data is left alone (passwords still stable); only
metadata is refreshed, so the build-aware cleanup correctly recognises
it as current.

Other stable-name resources (Services, Deployments, StatefulSets,
ConfigMaps, Ingresses, NetworkPolicies, Jobs) are not affected: their
apply paths do a full `Update` on `AlreadyExists`, so their labels
already get rewritten every Apply.

## Migration

None required for the code change. Any deployment whose
knowledge-creds Secret was already deleted by a previous broken redeploy
will need its knowledge StatefulSet pod recreated so it bootstraps
against the freshly generated password (the data inside the running pod
no longer matches anything the new Secret can hold). Easiest path in
local dev: delete the namespace and redeploy.
