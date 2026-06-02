#!/bin/bash
set -euo pipefail

NAME=${1:?usage: $(basename "$0") <blueprint-name> [host-port]}
HOST_PORT=${2:-18090}
AGENT="${NAME}-agent"

# Pin to docker-desktop for the local web-experience workflow. Override with
# AGENT_K8SFWD_CONTEXT=<name> if you need to forward against e.g. kind-e2e-host.
# We never mutate the user's active context with `use-context`.
CONTEXT=${AGENT_K8SFWD_CONTEXT:-docker-desktop}

# Kill only forwards pointing at this agent's pods; leave unrelated ones alone.
pkill -f "kubectl --context ${CONTEXT} port-forward .*${AGENT}-.*:8090" 2>/dev/null || true

# `grep` returning 1 on no-match would trip `set -o pipefail` and exit the
# script silently; capture pods first, then filter without pipefail in play.
PODS=$(kubectl --context "${CONTEXT}" get pods --all-namespaces --no-headers 2>/dev/null || true)
if [[ -z "$PODS" ]]; then
  echo "no pods reachable on context '${CONTEXT}'" >&2
  echo "  is the context available? kubectl config get-contexts" >&2
  echo "  override with: AGENT_K8SFWD_CONTEXT=<name> $(basename "$0") ..." >&2
  exit 1
fi
ROW=$(echo "$PODS" | { grep "$AGENT" || true; } | { grep Running || true; } | tail -1)
if [[ -z "$ROW" ]]; then
  echo "no running pod matching '${AGENT}' on context '${CONTEXT}'" >&2
  echo "  override the context with: AGENT_K8SFWD_CONTEXT=<name> $(basename "$0") ${NAME}" >&2
  exit 1
fi

NS=$(echo "$ROW" | awk '{print $1}')
POD=$(echo "$ROW" | awk '{print $2}')

# Port 8090 is the messaging sidecar's HTTP/SSE port (the web-experience
# endpoint). Default host port 18090 avoids the common 8090 collision with
# locally-published Docker containers (Traefik, etc.). Pass a second arg to
# override the host port.
LOG=$(mktemp -t agent-k8sfwd.XXXXXX)
kubectl --context "${CONTEXT}" port-forward "pod/${POD}" -n "${NS}" "${HOST_PORT}:8090" >"$LOG" 2>&1 &
PID=$!
disown

# Wait briefly for the forward to come up (or fail). kubectl prints
# "Forwarding from ..." to stdout once the local listener is bound.
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "port-forward exited immediately:" >&2
    sed 's/^/  /' "$LOG" >&2
    rm -f "$LOG"
    exit 1
  fi
  if grep -q "Forwarding from" "$LOG" 2>/dev/null; then
    break
  fi
  sleep 0.2
done

cat <<EOF
agent messaging port-forward up
  context:   ${CONTEXT}
  namespace: ${NS}
  pod:       ${POD}
  forward:   pod:8090 (messaging HTTP/SSE) -> http://localhost:${HOST_PORT}
  pid:       ${PID}
  log:       ${LOG}
  stop:      kill ${PID}
EOF
