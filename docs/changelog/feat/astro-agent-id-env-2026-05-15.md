---
title: ASTRO_AGENT_ID injected into every deployment container
---

## Summary

Every container astro-server creates for a deployment now sees its deployment ID as `ASTRO_AGENT_ID`. Previously only the collector sidecar received the deployment ID (as `ASTRO_DEPLOYMENT_ID`); the agent, messaging, and ingestion containers had no way to reference their own deployment without out-of-band wiring.

## Design

The deployment ID is plumbed through `deployment.ResolveContext` (new `DeploymentID` field) and surfaced in `ResolveDeploymentSpecEnv` as a ConfigMap entry:

```
ASTRO_AGENT_ID = <deployment id>
```

The agent ConfigMap is the env source for the agent container and ingestion job/cronjob containers, so a single ConfigMap entry covers both roles. The collector and messaging sidecars build their env explicitly without reading the agent ConfigMap, so each gets a dedicated `ASTRO_AGENT_ID` entry — the collector in `buildCollectorContainer` (alongside the existing `ASTRO_DEPLOYMENT_ID`) and the messaging sidecar via a new `DeploymentID` field on `MessagingDeploymentConfig`.

For parity, `deployment.Resolve` (used by the deployer to persist `deployment_build_env` rows) emits an `ASTRO_AGENT_ID` row per role present in the resolution, excluding `knowledge:*` roles whose containers run stock provider images shared across deployments and have no notion of a single owning deployment.

All four `ResolveDeploymentSpecEnv` call sites — `k8s/spec_applier.go`, `handlers/deploy.go`, `deploymentstore/normalized.go`, and `admingrpc/server.go` (BackfillResolvedKeys) — pass the deployment ID they already have in scope.

## Migration

No action required. New deployments include `ASTRO_AGENT_ID` automatically; existing deployments pick it up on next rollout. `ASTRO_DEPLOYMENT_ID` continues to be set on the collector for backward compatibility with the OTel astro processor.
