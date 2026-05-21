# Deployment status: container readiness vs Active badge

## Summary

The agent detail page showed **Active** (green) while workload containers were still starting during redeploys. Deployment-level `ready`/`replicas` from the agent component could reach 1/1 before every container in the agent pod (e.g. messaging sidecar) reported ready, so the history tile and dashboard badge disagreed with pod tiles showing **Starting**.

## Design

`hasContainerMismatch` — already used by `useDeployment` to keep polling during pause/resume and rollout windows — is now the single source of truth in `deployment-utils.ts`. `mapDeploymentStatus` returns **Deploying** when any Deployment/StatefulSet container is not ready (or pause/resume replica mismatch), instead of **Active**. Job/CronJob workloads remain excluded from the check (their health is on `wl.status`, not `containers[].ready`).

## Investigation notes

Reported in prod (Slack thread, #1116): after redeploy, the UI looked healthy for minutes while `@mention` did not work. That gap is not one bug — several independent signals all read as “ready” to different parts of the product:

| Signal | What it actually means | Slack usable? |
|--------|------------------------|---------------|
| DB `active` | K8s manifests applied | No |
| K8s `ready` / `readyReplicas` | Agent Deployment pod Ready (agent component only) | No |
| UI **Active** (before this PR) | Derived from deployment-level `ready`/`replicas` | No |
| SSE log `event: ready` | Log stream handshake, not workload health | No |
| Bridge “Agent ready” | gRPC stream registered with messaging | Only if Socket Mode is also connected |

**Slack traffic-ready** requires Socket Mode connected *and* an active agent gRPC stream. Nothing in the stack gates UI on that today.

This PR closes one slice of the mismatch: the client already polled on container readiness via `hasContainerMismatch`, but the badge ignored it. Top-level `ready`/`replicas` also comes from the **agent** Deployment only, while the API returns other workloads (collector, knowledge StatefulSets, etc.) whose containers can still be starting — or pause/resume can leave replica counts and container readiness briefly out of sync.

That still leaves the follow-up work below: honest copy for logs and toggles, waiting for rollout before DB `active`, exposing messaging traffic readiness on the API, and gating **Active** on Slack-specific readiness where applicable.

## Migration

None.

## Follow-up (#1116)

PR 1 of a stacked fix for [agent readiness UX](https://github.com/astropods/astro/issues/1116). Still planned separately:

- Log stream `event: ready` copy vs K8s readiness
- False green on pause/resume toggle semantics (`AgentStatusToggle`)
- DB `active` before K8s rollout completes
- Application-level readiness (Slack Socket Mode, agent gRPC stream)
- List/dashboard badges (`listAstroDeploymentsLight` has no workloads for container checks)
