#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure .env exists
if [ ! -f .env ]; then
  echo "==> No .env found, copying .env.example..."
  cp .env.example .env
fi

# Ensure air is installed
if ! command -v air &>/dev/null; then
  echo "==> Installing air (hot reload)..."
  go install github.com/air-verse/air@latest
fi

cleanup() {
  echo ""
  echo "==> Shutting down..."
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

# Run migrations (idempotent — safe to re-run)
echo "==> Running migrations..."
docker compose run --rm migrate

# Start the server with hot reload
echo "==> Starting astro-server (hot reload via air)..."
air &
SERVER_PID=$!
wait "$SERVER_PID"
