# Fix per-deployment network metrics selector

## Summary

The deployment-scoped network metrics endpoints introduced in #1123 (and surfaced on the agent Monitor page in #1129) were querying a Prometheus label that Beyla never emits, so every panel returned zero. This PR aligns the selector with the labels Beyla actually produces.

## Design

**Root cause.** The handler scoped every PromQL selector with `agent="<account>.<agent-name>"`, but our Beyla deployment runs with only the default k8s decoration (`attributes.kubernetes.enable: true`). That mode emits `k8s_namespace_name`, `k8s_pod_name`, `k8s_deployment_name`, and `service_name` — it does *not* promote arbitrary pod labels like `astro.dev/agent`. The `agent` label that exists on `messaging_*` series is set by the messaging app's own Prometheus client and is unrelated to Beyla.

**Switched to `k8s_namespace_name` + `service_name`.** This is exactly what the OBI Grafana dashboard (`obi-tenant-red.json`) filters on, so we're matching a query pattern already validated in production:

```
{__name__=~"http_server_request_duration_seconds_count", k8s_namespace_name="<ns>", service_name="<svc>", cluster="..."}
```

The pair is account-qualified: namespaces are per-account in Astro and `service_name` comes from the `app.kubernetes.io/name` pod label (set to `SanitizeName(agent)` in `naming.go`). Filtering by `service_name` alone would conflate agents with the same name across accounts; the namespace anchors the query to one tenant.

**Implementation.** `deploymentContext` replaces `AgentLabel` with `Namespace` + `ServiceName`, populated from `dep.Namespace` and `deployment.SanitizeName(dep.AgentName)`. `nameSelector` now emits the two-label matcher; all three handlers (`fillDirectionSummary`, `collectFlows`, `buildTimeseriesQL`) thread the new pair through. Tests updated to assert the new selector shape.

## Migration

None — no API contract change, no infra change. The agent Monitor page will start showing real network data on the next deploy.
