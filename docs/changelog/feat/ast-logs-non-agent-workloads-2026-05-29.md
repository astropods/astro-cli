# `ast agent logs` for non-agent workloads

## Summary

`ast agent logs` was hardcoded to the `agent` workload and refused any container name other than `app` or `messaging`. That left agents with knowledge or ingestion sidecars effectively un-debuggable from the CLI: a misconfigured collector, a crash-looping knowledge container, or any sidecar that didn't make it to the agent's view was invisible without direct cluster access. The server-side log endpoint already accepted any `workload` + `container` pair and resolved the pod from labels — only the client gated it.

## Design

Two small changes to the `agent logs` command in `apps/astro-cli/cmd/agent.go`:

1. **New `--workload` flag.** Accepts an exact workload name (`my-agent-knowledge-vectors`), an entry-name suffix (`vectors`), or a component label (`agent`, `messaging`, `collector`). When unset, behavior is unchanged — defaults to the agent workload. Ambiguous suffixes (e.g. `knowledge` when several knowledge entries exist) return an error that lists candidate names.

   ```
   ast agent logs my-agent --workload chat-sandbox --container app
   ast agent logs my-agent --workload my-agent-knowledge-chat-sandbox --container app
   ast agent logs my-agent --workload collector --container app
   ```

2. **Drop the `app`/`messaging` container whitelist.** The CLI now requires `--container` to be non-empty and delegates validation to the server, which already cross-checks against the pod's actual container list (`apps/astro-server/handlers/deploy.go:2884`). This is the existing contract used by the dashboard.

`ast agent get` is also updated to print each workload's K8s name next to its component (`knowledge (my-agent-knowledge-chat-sandbox)`) so users can discover what to pass to `--workload`.

The server-side API and label conventions are untouched.

## Migration

No action required. Existing usage (`ast agent logs <agent> --container app`) is unchanged. Scripts that relied on the CLI rejecting unknown container names will now have those requests go through to the server, which will return a 4xx if the container doesn't exist on the resolved pod.
