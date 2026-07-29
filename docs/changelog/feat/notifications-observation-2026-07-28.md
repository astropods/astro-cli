# Observation alerts

## Summary

Running agents can degrade silently *after* a successful deploy — OOM kills, crash loops, restart storms, unschedulable pods, resource pressure, or wasteful over-provisioning. Deploy-time failures already surface in the UI, but there was no path to detect sustained *runtime* problems and tell the user. This adds an in-process observation evaluator that detects those conditions, holds firing state, and delivers notifications, plus a per-workload Alerts view on the deployment page.

## Design

- **Evaluator (`internal/observation`).** A periodic sweep (every 5m) runs each condition's threshold query against metrics, tracks firing state in Postgres, and emits only on a firing *edge* after a sustained `for` window — so transient spikes and repeated sweeps never mail. This is a ~150-line evaluator over the existing `promquery` client rather than an embed of Prometheus's rules engine (which pulls ~320 modules into a server with no Prometheus deps).

- **Query engines.** Each condition declares an `Engine`; a `Querier` seam routes it to that backend (PromQL over VictoriaMetrics today, Langfuse later) so a new signal source is a registration, not an evaluator change.

- **Two workflows by severity.** Every condition collapses to one of two Novu workflows — `observation.critical` ("Agent failing") or `observation.warning` ("Agent degraded") — with the specific condition carried in the payload `reason`. The user gets two preference toggles, not one per condition, and adding a condition needs no new workflow. Both default to in-app with email opt-in.

- **Workload-aware state.** Firing state is keyed per `(deployment, workload, condition)`: the query groups `by (namespace, pod)` and the evaluator resolves each breaching pod to its workload (the `app.kubernetes.io/component`) via the runtime snapshot. The UI attributes an alert to the failing workload, while notifications stay **one per (deployment, condition) episode** (the `reason` names the workload) so per-workload state doesn't multiply mail.

- **Conditions** (all VM/Prom): `crash_loop`, `oom_killed`, `restart_storm`, `unschedulable`, `memory_over_budget`, `compute_over_budget`, and the right-sizing pair `cpu_over_provisioned` / `memory_over_provisioned` (usage far below request, sustained 6h).

- **UI.** An **Alerts** tab on the deployment pod panel, beside Events, renders the full condition catalog and each condition's live state (`ok` / `pending` / `firing`) for the open workload, Grafana-rules style: a state pill, a `for <age>` duration, and a severity chip.

## Migration

Adds the `deployment_alert_state` table (applied via Atlas). The two Novu workflows `observation.critical` and `observation.warning` must exist in the Novu environment. No user action required.
