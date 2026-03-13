#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure .env exists
if [ ! -f .env ]; then
  echo "==> No .env found, copying .env.example..."
  cp .env.example .env
fi

# Default ENVIRONMENT to "local" for dev
export ENVIRONMENT="${ENVIRONMENT:-local}"

# Ensure air is installed
if ! command -v air &>/dev/null; then
  echo "==> Installing air (hot reload)..."
  go install github.com/air-verse/air@latest
fi

cleanup() {
  echo ""
  echo "==> Shutting down..."
  if [ -n "${FAKEMETER_PID:-}" ]; then
    kill "$FAKEMETER_PID" 2>/dev/null
    wait "$FAKEMETER_PID" 2>/dev/null || true
  fi
  if [ -n "${SERVER_PID:-}" ]; then
    kill "$SERVER_PID" 2>/dev/null
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  docker compose down
  echo "==> Done."
}
trap cleanup EXIT INT TERM

# Start Postgres (reuses existing volume if present)
echo "==> Starting Postgres..."
docker compose up -d --wait postgres

# Apply schema via Atlas (idempotent — safe to re-run)
echo "==> Applying schema..."
docker compose run --rm migrate

# Apply River queue migrations (idempotent — CREATE IF NOT EXISTS)
echo "==> Applying River migrations..."
docker compose run --rm migrate-river

# Start fake OpenMeter if OPENMETER_URL points to localhost
if grep -q 'OPENMETER_URL=http://localhost:8888' .env 2>/dev/null; then
  echo "==> Starting fake OpenMeter on :8888..."
  go run ./cmd/fakeopenmeter &
  FAKEMETER_PID=$!
  sleep 1
fi

# Start the server with hot reload
echo "==> Starting astro-server (hot reload via air)..."
air &
SERVER_PID=$!
wait "$SERVER_PID"
