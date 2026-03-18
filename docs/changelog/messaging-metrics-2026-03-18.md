# Messaging Metrics Collection

## Summary

Exposes the Prometheus metrics port on the messaging sidecar container so that Grafana Alloy can scrape per-agent request counts, drop rates, and latency from the managed cluster.

## Design

The messaging container already exports Prometheus metrics at `:9091/metrics` — `messaging_messages_received_total`, `messaging_messages_forwarded_total`, `messaging_messages_dropped_total`, `messaging_message_latency_seconds`, and `messaging_active_streams`. The only missing piece was that port 9091 was not declared in the Kubernetes container port list, making it invisible to cluster-level scraping infrastructure.

Adding the `metrics` port (9091/TCP) to `buildMessagingContainer` in `deployment.go` is the only change needed in this repo. Alloy scrapes pod IPs directly, so the port declaration is informational but follows convention and makes the intent explicit.

The collection pipeline lives in astro-infra: Alloy discovers pods labeled `app.kubernetes.io/managed-by: astro-server`, filters to the `messaging` container, scrapes `:9091/metrics`, promotes the `astro.dev/agent` pod label to an `agent` metric label, and remote-writes to the infra Prometheus cluster. A Grafana dashboard (`Agent Messaging`) surfaces request rate by agent and platform, latency percentiles, drop reasons, and active stream count.

## Migration

No action required for existing deployments — the metrics endpoint was already running. Re-deploying an agent will pick up the updated pod spec with the port declaration.
