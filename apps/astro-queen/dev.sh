#!/usr/bin/env bash
# Hot-reloadable dev server for astro-queen
# Runs Go backend (air) + React frontend (vite) concurrently
set -euo pipefail

cd "$(dirname "$0")"

# Check air is installed
if ! command -v air &>/dev/null; then
  echo "Installing air..."
  go install github.com/air-verse/air@latest
fi

# Install web deps if needed
if [ ! -d web/node_modules ]; then
  echo "Installing web dependencies..."
  (cd web && bun install)
fi

cleanup() {
  echo ""
  echo "Shutting down..."
  kill 0 2>/dev/null
}
trap cleanup EXIT

echo "Starting astro-queen dev server..."
echo "  Go backend:  http://127.0.0.1:8888 (air - auto-reload)"
echo "  React frontend: http://127.0.0.1:5173 (vite - HMR)"
echo ""

# Run air (Go) and vite (React) concurrently
air &
(cd web && bun run dev) &

wait
