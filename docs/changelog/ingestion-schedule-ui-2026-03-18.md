# Ingestion UI, Local-Mode Security, and Knowledge Service Fixes

## Summary

The deploy and configure pages now fully support ingestion management — schedule triggers get a preset/custom cron picker, manual triggers get one-click run buttons. A round-trip bug where deployed values were lost on configure-page reload has been fixed server-side.

On the infrastructure side, third-party provider containers (Neo4j, Qdrant, etc.) were crashing in local K8s due to Pod Security Standards forcing `runAsUser: 1000` and `readOnlyRootFilesystem`. A new `LocalMode` flag conditionally relaxes hardening for provider containers only in local environments. Knowledge base Services now expose all declared endpoints (not just the primary port), fixing `ECONNREFUSED` when agents connect to secondary ports like Neo4j's Bolt (7687). Ingestion Jobs and CronJobs now respect the applier's `imagePullPolicy` instead of hardcoding `PullAlways`.

## Design

### Client — Ingestion UI

**Schedule picker** — Agents declaring `trigger: { type: schedule }` in `astropods.yml` see an "Ingestion" section on deploy/configure pages. A Select dropdown offers six presets (every 15 min, hourly, daily, etc.) plus a "Custom schedule" option that reveals five per-field selectors assembling a cron expression with a human-readable preview.

**Manual trigger buttons** — The configure page reads `deployment.manual_ingestions` and renders a button per entry. Each button calls `POST /api/v1/deployments/:id/ingestion/:name/trigger` via `useTriggerIngestion`, with spinner/check/error states.

**Form state wiring** — `useDeployForm` derives `scheduleIngestions` from the template, manages `ingestionSchedules` state, merges values into `fulfillTemplate`, and validates non-empty cron. `useChangeTracking` tracks schedules as a redeploy-category change. `extractInitialValues` reads prefilled cron from configure-page templates.

**Server-side prefill fix** — `GetPrefilledDeploymentTemplate` now also merges `ingestion.*.trigger.schedule` from the stored deployment spec, not just variables and adapters.

### Server — LocalMode Security Relaxation

`ApplierConfig.LocalMode` propagates from `K8sClientMode == "local"` through the deployer into every resource builder. When true, `hardenContainer` and `hardenPodSpec` are skipped for:

- **StatefulSets** (knowledge providers like Neo4j, Qdrant)
- **Deployments** where `Provider != ""` (model/knowledge provider containers)

Agent containers, sidecars (messaging, collector), and ingestion jobs are always hardened regardless of mode. This is enforced by the conditional placement: StatefulSets check `!cfg.LocalMode`, Deployments check `!(cfg.LocalMode && isProvider)`.

### Server — Knowledge Service Multi-Port

`buildKnowledgeService` was creating a single `ServicePort` from the primary endpoint. Providers like Neo4j declare both `http:7474` and `bolt:7687` as endpoints, but only 7474 was exposed. The Service now iterates all entries in `knowledge.Endpoints` and creates a `ServicePort` for each, sorted by name for deterministic output.

### Server — ImagePullPolicy for Ingestion

`buildIngestionContainer` accepted no pull policy and hardcoded `PullAlways`. `JobConfig` and `CronJobConfig` now carry an `ImagePullPolicy` field, defaulting to `PullAlways` when empty. The applier and `TriggerIngestion` handler both pass the mode-appropriate policy. `BuildIngestionDeployment` takes one fewer parameter as a result.

## Test Coverage

**Client E2E** — Schedule deploy (preset, custom, validation), configure-page edit, three round-trip tests (variable, schedule, combined), manual trigger visibility based on `manual_ingestions`, full trigger-then-navigate flow asserting the running ingestion pod on the detail page.

**Server unit** — `TestLocalModeIsolation_StatefulSet` and `_Deployment` (table-driven, local vs non-local x provider vs agent), sidecar hardening always applied, ingestion never affected, zero-value default, `Applier` propagation. `TestK8sClientModeLocalModeBoundary` for deployer mapping.

**Server E2E** — `TestE2E_KnowledgeService_ExposesAllProviderPorts` verifying Neo4j (http+bolt), Qdrant (http+grpc), and Redis Services expose all declared ports. CronJob schedule correctness. Handler tests for prefill schedule merge and ingestion trigger payload.

## Migration

No action required. LocalMode activates automatically when `K8sClientMode` is `"local"` — preview and production environments are unaffected. The multi-port Service change is additive (exposes ports the containers already listen on). Existing deployments without ingestion triggers see no difference.
