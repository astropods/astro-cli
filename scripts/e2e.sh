#!/usr/bin/env bash
# Run e2e integration tests locally.
#
# Usage:
#   ./scripts/e2e.sh          # setup infra + run all e2e tests
#   ./scripts/e2e.sh setup    # just create the cluster, don't run tests
#   ./scripts/e2e.sh teardown # destroy the cluster and Postgres container
#
# Prerequisites: docker, kubectl, go
# The script installs kind and vcluster if missing.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
KIND_CLUSTER="e2e-host"
VCLUSTER_NAME="e2e-test"
KUBECONFIG_PATH="/tmp/e2e-vcluster-kubeconfig.yaml"
PG_CONTAINER="e2e-postgres"
DATABASE_URL="postgres://postgres:postgres@localhost:5433/astro_e2e?sslmode=disable"
DEV_URL="postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable"

# --- helpers ---

info()  { printf "\033[1;34m==> %s\033[0m\n" "$*"; }
ok()    { printf "\033[1;32m==> %s\033[0m\n" "$*"; }
err()   { printf "\033[1;31m==> %s\033[0m\n" "$*" >&2; }

ensure_tool() {
  local name="$1" install_cmd="$2"
  if ! command -v "$name" &>/dev/null; then
    info "Installing $name..."
    eval "$install_cmd"
  fi
}

# --- postgres ---

setup_postgres() {
  if docker ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
    info "Postgres already running ($PG_CONTAINER)"
    return
  fi

  if docker ps -a --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
    info "Starting existing Postgres container..."
    docker start "$PG_CONTAINER"
  else
    info "Creating Postgres container on port 5433..."
    docker run -d --name "$PG_CONTAINER" \
      -e POSTGRES_USER=postgres \
      -e POSTGRES_PASSWORD=postgres \
      -e POSTGRES_DB=astro_e2e \
      -p 5433:5432 \
      --health-cmd "pg_isready -U postgres" \
      --health-interval 2s \
      --health-timeout 3s \
      --health-retries 10 \
      postgres:16-alpine
  fi

  info "Waiting for Postgres..."
  for i in $(seq 1 30); do
    if docker exec "$PG_CONTAINER" pg_isready -U postgres &>/dev/null; then
      ok "Postgres ready"
      return
    fi
    sleep 1
  done
  err "Postgres did not become ready"
  exit 1
}

migrate_postgres() {
  info "Applying schema migrations..."
  atlas schema apply \
    --url "$DATABASE_URL" \
    --to "file://${ROOT}/sql/astro-server/schema.sql" \
    --dev-url "$DEV_URL" \
    --exclude "atlas_schema_revisions,river" \
    --auto-approve

  atlas migrate apply \
    --url "$DATABASE_URL" \
    --dir "file://${ROOT}/sql/river" \
    --revisions-schema atlas_schema_revisions \
    --allow-dirty

  ok "Migrations applied"
}

# --- kind + vcluster ---

setup_cluster() {
  ensure_tool kind 'brew install kind'
  ensure_tool vcluster 'brew install loft-sh/tap/vcluster'

  # Kind cluster
  if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
    info "Kind cluster '$KIND_CLUSTER' exists"
  else
    info "Creating kind cluster '$KIND_CLUSTER'..."
    kind create cluster --name "$KIND_CLUSTER" --wait 60s
  fi

  # Switch kubectl to kind context
  kubectl cluster-info --context "kind-${KIND_CLUSTER}" &>/dev/null || {
    err "Cannot reach kind cluster"
    exit 1
  }

  # vcluster
  if vcluster list 2>/dev/null | grep -q "$VCLUSTER_NAME"; then
    info "vcluster '$VCLUSTER_NAME' exists"
  else
    info "Creating vcluster '$VCLUSTER_NAME'..."
    vcluster create "$VCLUSTER_NAME" --connect=false || true
  fi

  info "Connecting to vcluster..."
  vcluster connect "$VCLUSTER_NAME" --update-current=false --kube-config="$KUBECONFIG_PATH"

  info "Waiting for vcluster API server..."
  for i in $(seq 1 30); do
    if kubectl --kubeconfig="$KUBECONFIG_PATH" cluster-info &>/dev/null; then
      ok "vcluster ready"
      return
    fi
    sleep 2
  done
  err "vcluster did not become ready"
  exit 1
}

# --- teardown ---

teardown() {
  info "Tearing down e2e infrastructure..."

  if command -v vcluster &>/dev/null && vcluster list 2>/dev/null | grep -q "$VCLUSTER_NAME"; then
    info "Deleting vcluster..."
    vcluster delete "$VCLUSTER_NAME" || true
  fi

  if command -v kind &>/dev/null && kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
    info "Deleting kind cluster..."
    kind delete cluster --name "$KIND_CLUSTER"
  fi

  if docker ps -a --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
    info "Removing Postgres container..."
    docker rm -f "$PG_CONTAINER"
  fi

  rm -f "$KUBECONFIG_PATH"
  ok "Teardown complete"
}

# --- run tests ---

run_tests() {
  info "Running e2e tests..."
  cd "$ROOT/apps/astro-server"

  KUBECONFIG="$KUBECONFIG_PATH" \
  DATABASE_URL="$DATABASE_URL" \
    go test -tags k8s -race -timeout 5m -v ./e2e/...
}

# --- main ---

case "${1:-all}" in
  setup)
    setup_postgres
    migrate_postgres
    setup_cluster
    ok "E2E infrastructure ready. Run: ./scripts/e2e.sh"
    ;;
  teardown)
    teardown
    ;;
  all)
    setup_postgres
    migrate_postgres
    setup_cluster
    run_tests
    ;;
  *)
    echo "Usage: $0 {all|setup|teardown}"
    exit 1
    ;;
esac
