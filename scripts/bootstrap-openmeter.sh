#!/usr/bin/env bash
# Idempotent bootstrap for OpenMeter: creates meters, features, and the private_beta plan.
# Only intended for local development — called by apps/astro-server/scripts/dev.sh when ENVIRONMENT=local.
# Reads OPENMETER_URL from the environment. Skips with a notice if unset.
set -euo pipefail

if [ -z "${OPENMETER_URL:-}" ]; then
  echo "==> OPENMETER_URL not set — skipping OpenMeter bootstrap."
  exit 0
fi

echo "==> Bootstrapping OpenMeter at $OPENMETER_URL..."

echo "==> Waiting for OpenMeter..."
for i in $(seq 1 30); do
  if curl -s "$OPENMETER_URL/api/v1/meters" > /dev/null 2>&1; then
    break
  fi
  [ "$i" -eq 30 ] && { echo "ERROR: OpenMeter not ready after 60s"; exit 1; }
  sleep 2
done
echo "==> OpenMeter ready."

# POST to an OpenMeter endpoint. Treats 200/201 as created, 409 as already exists.
om_post() {
  local label="$1"
  local path="$2"
  local payload="$3"
  local status
  status=$(curl -s -o /tmp/om_out -w "%{http_code}" \
    -X POST "$OPENMETER_URL$path" \
    -H "Content-Type: application/json" \
    -d "$payload" || echo "000")
  case "$status" in
    200|201|409) echo "  $label: ok" ;;
    *) echo "  ERROR $label: HTTP $status — $(cat /tmp/om_out 2>/dev/null)"; exit 1 ;;
  esac
}

# ── Meters ───────────────────────────────────────────────────────────────────

echo "==> Creating meters..."

om_post "compute" /api/v1/meters '{
  "slug": "compute",
  "name": "Compute Consumed",
  "description": "Compute-unit-hours consumed by active deployments, per container",
  "eventType": "compute_usage",
  "aggregation": "SUM",
  "valueProperty": "$.compute_unit_hours",
  "groupBy": { "agent_name": "$.agent_name", "namespace": "$.namespace", "component": "$.component" }
}'

om_post "agents" /api/v1/meters '{
  "slug": "agents",
  "name": "Active Agents",
  "description": "Number of distinct agents registered per account",
  "eventType": "active_agents",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}'

om_post "agent_builds" /api/v1/meters '{
  "slug": "agent_builds",
  "name": "Agent Builds",
  "description": "Number of agent builds pushed to the registry",
  "eventType": "agent_build",
  "aggregation": "COUNT",
  "groupBy": { "agent_name": "$.agent_name" }
}'

om_post "agent_deployments" /api/v1/meters '{
  "slug": "agent_deployments",
  "name": "Active Deployments",
  "description": "Number of currently active agent deployments per account",
  "eventType": "active_deployments",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}'

om_post "members" /api/v1/meters '{
  "slug": "members",
  "name": "Account Members",
  "description": "Number of members in each account",
  "eventType": "active_members",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}'

om_post "knowledge_stores" /api/v1/meters '{
  "slug": "knowledge_stores",
  "name": "Active Knowledge Stores",
  "description": "Total number of knowledge stores (managed + external) per account",
  "eventType": "active_knowledge_stores",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}'

om_post "knowledge_storage" /api/v1/meters '{
  "slug": "knowledge_storage",
  "name": "Knowledge Storage Provisioned",
  "description": "Provisioned storage in GB per managed knowledge store",
  "eventType": "knowledge_storage_provisioned",
  "aggregation": "LATEST",
  "valueProperty": "$.storage_gb",
  "groupBy": { "store_name": "$.store_name", "provider": "$.provider" }
}'

om_post "knowledge_compute" /api/v1/meters '{
  "slug": "knowledge_compute",
  "name": "Knowledge Compute",
  "description": "Compute-unit-hours consumed by managed knowledge store StatefulSets",
  "eventType": "knowledge_compute_usage",
  "aggregation": "SUM",
  "valueProperty": "$.compute_unit_hours",
  "groupBy": { "store_name": "$.store_name", "provider": "$.provider" }
}'

om_post "knowledge_endpoints" /api/v1/meters '{
  "slug": "knowledge_endpoints",
  "name": "Active Knowledge Endpoints",
  "description": "Number of active PrivateLink VPC endpoints per account",
  "eventType": "active_knowledge_endpoints",
  "aggregation": "LATEST",
  "valueProperty": "$.count"
}'

# ── Features ─────────────────────────────────────────────────────────────────

echo "==> Creating features..."

om_post "compute"             /api/v1/features '{"key":"compute","name":"Compute","meterSlug":"compute","metadata":{"unit":"CU-hours"}}'
om_post "agents"              /api/v1/features '{"key":"agents","name":"Agents","meterSlug":"agents","metadata":{"unit":"agents"}}'
om_post "agent_builds"        /api/v1/features '{"key":"agent_builds","name":"Agent Builds","meterSlug":"agent_builds","metadata":{"unit":"builds"}}'
om_post "agent_deployments"   /api/v1/features '{"key":"agent_deployments","name":"Deployments","meterSlug":"agent_deployments","metadata":{"unit":"deployments"}}'
om_post "members"             /api/v1/features '{"key":"members","name":"Members","meterSlug":"members","metadata":{"unit":"members"}}'
om_post "knowledge_stores"    /api/v1/features '{"key":"knowledge_stores","name":"Knowledge Stores","meterSlug":"knowledge_stores","metadata":{"unit":"stores"}}'
om_post "knowledge_storage"   /api/v1/features '{"key":"knowledge_storage","name":"Knowledge Storage","meterSlug":"knowledge_storage","metadata":{"unit":"GB"}}'
om_post "knowledge_compute"   /api/v1/features '{"key":"knowledge_compute","name":"Knowledge Compute","meterSlug":"knowledge_compute","metadata":{"unit":"CU-hours"}}'
om_post "knowledge_endpoints" /api/v1/features '{"key":"knowledge_endpoints","name":"Knowledge Endpoints","meterSlug":"knowledge_endpoints","metadata":{"unit":"endpoints"}}'

# ── Plan ─────────────────────────────────────────────────────────────────────

echo "==> Creating private_beta plan..."

om_post "private_beta" /api/v1/plans '{
  "key": "private_beta",
  "name": "Private Beta",
  "description": "Free tier for private beta users with hard limits on all resources.",
  "currency": "USD",
  "billingCadence": "P1M",
  "phases": [
    {
      "key": "beta",
      "name": "Private Beta",
      "description": "Single phase — free access with hard-capped entitlements.",
      "duration": null,
      "rateCards": [
        {
          "type": "usage_based", "key": "compute", "name": "Compute",
          "description": "Compute-unit-hours consumed by active deployments (1 CU = 1 vCPU + 2 GB RAM per hour).",
          "featureKey": "compute", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 100 }
        },
        {
          "type": "usage_based", "key": "agents", "name": "Agents",
          "description": "Number of distinct agents registered in the account.",
          "featureKey": "agents", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 5 }
        },
        {
          "type": "usage_based", "key": "agent_builds", "name": "Agent Builds",
          "description": "Number of agent builds pushed to the registry per billing period.",
          "featureKey": "agent_builds", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 50 }
        },
        {
          "type": "usage_based", "key": "agent_deployments", "name": "Deployments",
          "description": "Number of concurrently active agent deployments.",
          "featureKey": "agent_deployments", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 10 }
        },
        {
          "type": "usage_based", "key": "members", "name": "Members",
          "description": "Number of members in the account.",
          "featureKey": "members", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 5 }
        },
        {
          "type": "usage_based", "key": "knowledge_stores", "name": "Knowledge Stores",
          "description": "Number of knowledge stores (managed + external) per account.",
          "featureKey": "knowledge_stores", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 5 }
        },
        {
          "type": "usage_based", "key": "knowledge_storage", "name": "Knowledge Storage",
          "description": "Total provisioned storage (GB) across managed knowledge stores.",
          "featureKey": "knowledge_storage", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 50 }
        },
        {
          "type": "usage_based", "key": "knowledge_compute", "name": "Knowledge Compute",
          "description": "Compute-unit-hours consumed by managed knowledge store infrastructure.",
          "featureKey": "knowledge_compute", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 50 }
        },
        {
          "type": "usage_based", "key": "knowledge_endpoints", "name": "Knowledge Endpoints",
          "description": "Number of active PrivateLink VPC endpoints for external knowledge stores.",
          "featureKey": "knowledge_endpoints", "billingCadence": "P1M", "price": null,
          "entitlementTemplate": { "type": "metered", "isSoftLimit": false, "issueAfterReset": 2 }
        }
      ]
    }
  ]
}'

echo "==> OpenMeter bootstrap complete."
