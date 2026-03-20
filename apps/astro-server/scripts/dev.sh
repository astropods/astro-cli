#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# Ensure .env exists
if [ ! -f .env ]; then
  echo "==> No .env found, copying .env.example..."
  cp .env.example .env
fi

# Load DATABASE_URL from .env
DATABASE_URL=$(grep '^DATABASE_URL=' .env | cut -d= -f2-)
if [ -z "$DATABASE_URL" ]; then
  echo "ERROR: DATABASE_URL not set in .env"
  exit 1
fi

# Default ENVIRONMENT to "local" for dev
export ENVIRONMENT="${ENVIRONMENT:-local}"

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
  echo "==> Done."
}
trap cleanup EXIT INT TERM

# Apply schema via Atlas (idempotent — safe to re-run)
echo "==> Applying schema..."
atlas schema apply \
  --url "$DATABASE_URL&search_path=public" \
  --to "file://../../sql/astro-server/schema.sql" \
  --dev-url "docker://postgres/16/dev?search_path=public" \
  --exclude atlas_schema_revisions \
  --exclude river \
  --auto-approve

# Apply River queue migrations (idempotent — CREATE IF NOT EXISTS)
echo "==> Applying River migrations..."
atlas migrate apply \
  --url "$DATABASE_URL" \
  --dir "file://../../sql/river" \
  --revisions-schema atlas_schema_revisions \
  --allow-dirty

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
