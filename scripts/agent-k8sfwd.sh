#!/bin/bash
NAME=${1:?usage: $(basename "$0") <name>}
AGENT="${NAME}-agent"
pkill -f "kubectl port-forward" 2>/dev/null
sleep 1
ROW=$(kubectl get pods --all-namespaces --no-headers | grep "$AGENT" | grep Running | tail -1)
NS=$(echo $ROW | awk '{print $1}')
POD=$(echo $ROW | awk '{print $2}')
echo "Forwarding $POD ($NS):8090 -> localhost:8090"
kubectl port-forward pod/"$POD" -n "$NS" 8090:8090 &
