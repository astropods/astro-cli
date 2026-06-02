#!/bin/bash
set -euo pipefail

NAME=${1:?usage: $(basename "$0") <blueprint-name> [host-port]}
HOST_PORT=${2:-18090}
AGENT="${NAME}-agent"

# Kill only forwards pointing at this agent's pods; leave unrelated ones alone.
pkill -f "kubectl port-forward .*${AGENT}-" 2>/dev/null || true

ROW=$(kubectl get pods --all-namespaces --no-headers 2>/dev/null \
  | grep "$AGENT" | grep Running | tail -1)
if [[ -z "$ROW" ]]; then
  echo "no running pod matching '${AGENT}' in any namespace" >&2
  echo "  current context: $(kubectl config current-context 2>/dev/null || echo unknown)" >&2
  exit 1
fi

NS=$(echo "$ROW" | awk '{print $1}')
POD=$(echo "$ROW" | awk '{print $2}')

# Default host port 18090 avoids the common 8090 collision with locally-published
# Docker containers (Traefik, etc.). Pass a second arg to override.
echo "Forwarding ${POD} (${NS}):8090 -> localhost:${HOST_PORT}"

# Discard kubectl's async output so its "Forwarding from..." line doesn't
# land on the shell prompt; disown so the shell doesn't report job state
# changes later either. Trade-off: if the forward errors out, you'll need
# to re-run rather than read a saved log.
kubectl port-forward "pod/${POD}" -n "${NS}" "${HOST_PORT}:8090" >/dev/null 2>&1 &
disown
