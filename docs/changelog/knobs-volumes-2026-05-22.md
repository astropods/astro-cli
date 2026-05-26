# Provisioning knobs and agent volumes

## Summary

Deployers can now set CPU, memory, and a persistent volume per
deployment from the UI or the API. The deploy flow's integrity model
is simplified to a single check: signed templates.

Before this change there was no way to size an agent — every container
got the same `StandardResources` defaults, and only knowledge stores
could request a persistent volume. There was also a separate
`editable` allowlist on the deployment template that the deploy
handler used as a fallback integrity check whenever a signature was
missing.

## Design

### Provisioning input on the template endpoint

`POST /api/v1/agents/:account/:name/deployment-template` accepts a new
top-level `provisioning` block scoped to the agent in v1:

```yaml
provisioning:
  agent:
    compute:
      cpu: "100m"     # or 25m, 50m, 250m, 500m, 1
      memory: "2Gi"
    volume:
      mount: /data
      storage:
        size: 20Gi
        class: gp3
```

The server applies these on top of `StandardResources` and
`DefaultStorageConfig()` per field. `ComponentCompute` deliberately
exposes only `cpu` and `memory` so the UI stays DigitalOcean-simple;
the server expands these into K8s requests == limits so pods land in
Guaranteed QoS without users having to think about burstability.

`TemplateResponse.provisioning` echoes the resolved values so
clients can render sizing controls from the same shape they post.

### Agent volumes

`DeploymentAgent` gains `volume` + `storage` fields, matching what
`DeploymentKnowledge` already had. A non-empty `volume` switches the
agent from a Deployment to a StatefulSet with a PVC; the shared
StatefulSet builder learns to mount the messaging sidecar plus
`ExtraEnv` / `ExtraSecretNames` so the persistent path keeps the agent's
knowledge-cred refs and authz token. The choice between Deployment and
StatefulSet is extracted into `applyAgentWorkload` so the top-level
applier stays a single intent line.

The agent's StatefulSet sets
`PersistentVolumeClaimRetentionPolicy: { WhenDeleted: Delete,
WhenScaled: Delete }`. Without this, toggling the volume off on
redeploy would orphan the PVC (the stale-cleanup pass deletes the
StatefulSet by build label but never touches PVCs), leaving the
disk billing forever. Knowledge stores keep their existing
`Retain/Delete` policy because their data is opt-in shared across
deployments.

### Editable is gone; signature is mandatory

`Editable` and `EnforceEditable` were a band-aid for clients that
submitted unsigned specs — the deploy handler would regenerate the
template and diff against an allowlist. With explicit provisioning
input, the regenerate-and-diff path is redundant: the spec the client
submits is whatever the template endpoint resolved, and signature
verification (HMAC-SHA256 over the canonical spec, target fields
excluded) is now the only integrity check.

The CLI and web client both request `finalize: true` on the template
POST and forward the resulting signature in the `X-Template-Signature`
header on `POST /deploy`. `/deploy/validate` skips the signature check
since no actual deploy happens there.

### UI

The deploy page gains a root-level **Volume** section and a
collapsible **Advanced** panel.

The Volume section is a checkbox-style card that mirrors the
interfaces picker (Web/Slack). Enabling it expands to reveal the
mount path Input; the storage size slider lives in the Advanced
panel and only renders once the volume is enabled. This split keeps
the disk-or-no-disk decision next to the other top-level deploy
choices (interfaces, knowledge, credentials) while keeping the size
knob alongside CPU and memory where it's priced.

The Advanced panel hosts CPU, memory, and (when the volume is on)
storage tier sliders. It surfaces a live cost estimate (placeholder
unit rates approximating AWS Fargate + EBS) broken down by CPU,
RAM, and storage, with the monthly total visible on the collapsed
header so users see the impact of every adjustment.

The storage ladder is `5 / 10 / 20 / 30 / 50 Gi` with `10Gi` as the
default, matching `DefaultStorageConfig()`. The slider is locked
on existing deployments whose volume is already provisioned, since
a live PVC cannot be resized in place — the StatefulSet's
`volumeClaimTemplates` is immutable on update and the applier
preserves the existing one. Removing the volume on redeploy
clears the PVC (see below), after which the user can re-enable at
a fresh size.

The CPU ladder is `25m / 50m / 100m / 250m / 500m / 1` with `100m`
as the default. The ceiling is intentional: real agent workloads
spend most of their time blocked on LLM and tool calls, so giving
the container a full core is already generous; offering 2/4-core
tiers would mostly invite over-provisioning. The memory ladder is
`256Mi / 512Mi / 1Gi / 2Gi / 4Gi` (default `1Gi`) — LLM client
libraries and tool runtimes are memory-heavier than they are
CPU-heavy, so the default lives a tier above CPU; the 8Gi
default-anchor trap is gone. Both axes accept manual overrides
via the `provisioning` API for the rare workload that needs more.

`StandardResources` is now `100m / 1Gi` with request == limit, so
the deployment-template echo lands on the same values the UI's
sliders default to and pods stay in Guaranteed QoS without any
provisioning input. `MessagingResources` (`100m / 256Mi`) and
`CollectorResources` (`50m / 128Mi`) follow the same pattern —
any Burstable container in the pod would drag the whole pod down
to Burstable QoS even when the agent is Guaranteed, so the
sidecars have to match.

### Metrics tab surfaces the limit

`GET /deployments/:id/pods/:pod/metrics` now returns
`cpu_limit` (vCPU cores) and `memory_limit` (bytes), summed
across the pod's regular containers and Always-restart init
sidecars. The metrics tab renders these as dashed reference
lines on the CPU and memory charts and pins each chart's Y-axis
to the limit, so a user can see headroom at a glance and tell
when an OOM-kill marker is imminent.

## Migration

- **Clients of `POST /deploy`** must obtain a signed template from
  `POST /deployment-template` with `finalize: true` and resubmit it
  verbatim with the `X-Template-Signature` header. Unsigned deploys are
  rejected. The bundled CLI and web client already do this.
- **`AstroDeploymentSpec.editable` and `TemplateResponse.editable` are
  removed.** Consumers that read them should switch to reading the
  resolved values directly off `template.agent` /
  `provisioning.agent`.
