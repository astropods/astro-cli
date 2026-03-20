#!/usr/bin/env bash
# Smoke test for the astro-client Docker image.
#
# Builds the production image and starts it, then waits for any HTTP response
# from the server. A successful response proves that server.ts started without
# crashing — meaning all production dependencies resolved correctly at import
# time. Missing deps (e.g. a package declared only as a workspace dep) would
# cause the process to exit before it ever binds to the port.
#
# Usage: sh deployment/smoke-test-astro-client.sh [--skip-build]
#   --skip-build  Skip docker build and test the existing astro-client:smoke image.

set -euo pipefail

SKIP_BUILD=false
for arg in "$@"; do
  [ "$arg" = "--skip-build" ] && SKIP_BUILD=true
done

IMAGE="astro-client:smoke"
CONTAINER="astro-client-smoke-$$"
PORT=13000

cleanup() {
  docker rm -f "$CONTAINER" 2>/dev/null || true
}
trap cleanup EXIT

# Always run from workspace root
cd "$(dirname "$0")/.."

if [ "$SKIP_BUILD" = false ]; then
  echo "==> Building $IMAGE..."
  docker build -t "$IMAGE" -f deployment/Dockerfile.astro-client .
fi

echo "==> Starting container (host port $PORT)..."
docker run -d \
  --name "$CONTAINER" \
  -p "$PORT:3000" \
  "$IMAGE"

echo "==> Waiting for server to accept connections..."
for i in $(seq 1 30); do
  HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:$PORT/" || true)
  if [ -n "$HTTP_STATUS" ] && [ "$HTTP_STATUS" != "000" ]; then
    echo "==> Server responded with HTTP $HTTP_STATUS"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: server did not respond within 30s"
    echo "--- container logs ---"
    docker logs "$CONTAINER"
    exit 1
  fi
  sleep 1
done

echo "==> Smoke test passed"
