#!/usr/bin/env bash
set -euo pipefail
# Job control: each backgrounded subshell becomes its own process group
# so cleanup can kill the entire subtree via `kill -- -$PID`. Without
# this, signaling the subshell PID leaves grandchildren (bash dev.sh,
# sed, bun run dev) orphaned to launchd.
set -m

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
log()  { echo -e "${GREEN}[local-dev]${NC} $*"; }
err()  { echo -e "${RED}[local-dev]${NC} $*" >&2; }

# ── preflight ────────────────────────────────────────────────────────────────

if [ ! -f apps/astro-server/.env ]; then
  log "apps/astro-server/.env not found. Create it (see apps/astro-server/.env.example) before running local-dev."
  exit 0
fi

if ! docker info &>/dev/null; then
  err "Docker is not running. Start Docker and try again."
  exit 1
fi

# ── traefik ───────────────────────────────────────────────────────────────────

log "Starting Traefik..."
docker compose -f docker-compose.local.yml up -d

# Bootstrap OpenMeter once on first local dev startup (idempotent — safe to re-run)
OPENMETER_URL=$(grep '^OPENMETER_URL=' apps/astro-server/.env 2>/dev/null | cut -d= -f2- | tr -d "\"'" | tr -d '[:space:]')
if [ -n "$OPENMETER_URL" ]; then
  OPENMETER_URL="$OPENMETER_URL" bash scripts/bootstrap-openmeter.sh
fi

# ── cleanup ──────────────────────────────────────────────────────────────────

PIDS=()
_cleaned_up=false

cleanup() {
  $_cleaned_up && return
  _cleaned_up=true
  echo ""
  log "Shutting down..."
  for pid in ${PIDS[@]+"${PIDS[@]}"}; do
    kill -- "-$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  docker compose -f docker-compose.local.yml down --timeout 5
  log "Done."
}
trap cleanup EXIT INT TERM

# ── dev servers ──────────────────────────────────────────────────────────────

log "Installing dependencies..."
bun install

log "Building ast-dev CLI..."
ASTRO_SERVER_URL=http://localhost moon run astro-cli:build
log "Built ast-dev → apps/astro-cli/bin/ast-dev"

log "Building workspace packages (astro-theme, astro-trading-card)..."
moon run astro-theme:build astro-trading-card:build

log "Starting astro-server (:8080)..."
(cd apps/astro-server && bash scripts/dev.sh 2>&1 | sed 's/^/[server] /') &
PIDS+=($!)

log "Starting astro-client (:5173 → localhost)..."
(cd apps/astro-client && bun run dev --host 2>&1 | sed 's/^/[client] /') &
PIDS+=($!)

# ── summary ──────────────────────────────────────────────────────────────────

echo ""
echo "┌────────────────────────────────────────────────┐"
echo "│               local-dev services               │"
echo "├────────────────────────────────────────────────┤"
echo "│  http://localhost            astro-client      │"
echo "│  http://localhost/api        astro-server      │"
echo "│  http://localhost:8090       Traefik dashboard │"
echo "│  apps/astro-cli/bin/ast-dev  local CLI         │"
echo "└────────────────────────────────────────────────┘"
echo ""
log "Press Ctrl+C to stop all services."

wait
