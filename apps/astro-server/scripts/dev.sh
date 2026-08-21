#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure .env exists
if [ ! -f .env ]; then
  echo "==> No .env found, copying .env.example..."
  cp .env.example .env
fi

# Read a key from .env, falling back to a default. Last assignment wins and
# surrounding quotes are stripped, so a hand-edited .env reads the same way
# godotenv reads it.
env_value() {
  local key="$1" default="${2:-}" value
  value=$(grep "^${key}=" .env | tail -n 1 | cut -d= -f2- || true)
  value="${value%\"}"; value="${value#\"}"
  value="${value%\'}"; value="${value#\'}"
  echo "${value:-$default}"
}

DATABASE_URL=$(env_value DATABASE_URL)
if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL not set in .env"
  exit 1
fi

# Default ENVIRONMENT to "local" for dev
export ENVIRONMENT="${ENVIRONMENT:-local}"

# Generate this machine's cluster config. astro-server reads every cluster's
# ingress domains and observability URLs from its cluster-config entry, and
# there is no env-var path to them, so local dev needs an entry of its own.
# It cannot be checked in: the cluster id, kube context, and backend URLs
# differ per developer. The EKS coordinates below are inert — a local cluster
# builds its client from kubeconfig — but boot sync requires them non-empty.
K8S_CLIENT_MODE=$(env_value K8S_CLIENT_MODE local)
CLUSTER_CONFIG_PATH=$(env_value CLUSTER_CONFIG_PATH)
if [ "$K8S_CLIENT_MODE" = "local" ] && [ -z "$CLUSTER_CONFIG_PATH" ]; then
  KUBE_CONTEXT=$(env_value KUBE_CONTEXT docker-desktop)
  CLUSTER_ID=$(env_value DEFAULT_CLUSTER_ID "$KUBE_CONTEXT")
  # Not tmp/: air empties that directory when it exits, and the file has to
  # outlive a dev session so `go run .` finds it too.
  CLUSTER_CONFIG_PATH="$PWD/.cluster-config.json"

  cat > "$CLUSTER_CONFIG_PATH" <<EOF
[
  {
    "id": "$CLUSTER_ID",
    "region": "$(env_value AWS_REGION local)",
    "eks_cluster_name": "$KUBE_CONTEXT",
    "eks_cluster_endpoint": "https://kubernetes.docker.internal:6443",
    "eks_cluster_ca": "",
    "agent_ingress_domain": "$(env_value INGRESS_DOMAIN agents.localtest.me)",
    "agent_public_ingress_domain": "$(env_value AGENT_INGRESS_PUBLIC_DOMAIN agents.public.localtest.me)",
    "ingestion_ingress_domain": "$(env_value INGESTION_INGRESS_DOMAIN ingestion.localtest.me)",
    "langfuse_base_url_ext": "$(env_value LANGFUSE_BASE_URL_EXT "$(env_value LANGFUSE_BASE_URL http://localhost:3000)")",
    "langfuse_vpce_ips": "",
    "pod_subnet_cidrs": "$(env_value POD_SUBNET_CIDRS 10.1.0.0/16)",
    "pod_subnet_ipv6_cidrs": "",
    "loki_url": "$(env_value LOKI_URL)",
    "prometheus_url": "$(env_value PROMETHEUS_URL)",
    "tenant_router_internal_url": ""
  }
]
EOF
  export CLUSTER_CONFIG_PATH DEFAULT_CLUSTER_ID="$CLUSTER_ID"
  echo "==> Generated cluster config for $CLUSTER_ID ($CLUSTER_CONFIG_PATH)"
fi

# Ensure air is installed
if ! command -v air &>/dev/null; then
  echo "==> Installing air (hot reload)..."
  go install github.com/air-verse/air@latest
fi

# Ensure atlas is installed
if ! command -v atlas &>/dev/null; then
  echo "==> Installing atlas..."
  curl -sSf https://atlasgo.sh | sh
fi

# Apply schema via Atlas (idempotent — safe to re-run)
echo "==> Applying schema..."
case "$DATABASE_URL" in *\?*) sep="&" ;; *) sep="?" ;; esac
atlas schema apply \
  --url "${DATABASE_URL}${sep}search_path=public" \
  --to "file://../../sql/astro-server/schema.sql" \
  --dev-url "docker://postgres/16/dev?search_path=public" \
  --exclude atlas_schema_revisions \
  --exclude river \
  --auto-approve

# Backfill undeployed_at for soft-deletes that predate the bugfix making
# UpdateStatus(StatusUndeployed) stamp the timestamp in the same UPDATE.
# Before the fix, the undeploy worker called MarkUndeployedByID after
# transitioning to 'undeployed', and that helper's `WHERE status='active'`
# guard always failed in the normal flow — so undeployed_at stayed NULL
# for every soft-delete. status_changed_at carries the same moment, so
# it's a safe proxy. Idempotent: after the first run the WHERE matches
# zero rows.
echo "==> Backfilling undeployed_at for legacy soft-deletes..."
psql "$DATABASE_URL" -c "UPDATE deployments SET undeployed_at = status_changed_at WHERE status = 'undeployed' AND undeployed_at IS NULL;" >/dev/null

# Apply River queue migrations (idempotent — CREATE IF NOT EXISTS)
echo "==> Applying River migrations..."
atlas migrate apply \
  --url "$DATABASE_URL" \
  --dir "file://../../sql/river" \
  --revisions-schema atlas_schema_revisions \
  --allow-dirty

# exec so air replaces bash and receives parent signals directly — no trap
# hop, so air has time to kill its astro-server child (which lives in its
# own process group via Setpgid:true) before we exit.
echo "==> Starting astro-server (hot reload via air)..."
exec air
