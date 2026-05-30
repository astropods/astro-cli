# `ast agent logs`: fold `--container` into `--workload`

## Summary

Follow-up to PR #1210. Two review comments asked us to (1) collapse `--workload` and `--container` into one flag since they always travel together, and (2) move the new user-facing error strings into `cmd/messages.go` where the rest of the CLI's copy lives.

## Design

`ast agent logs` now takes a single `--workload` flag whose value is `workload[/container]`. The workload portion resolves the same way as before — exact name, entry-name suffix, or component label — and an optional `/<container>` suffix selects a specific container inside the pod. When the suffix is omitted the server returns logs from all containers in the workload.

```
ast agent logs my-agent                          # agent workload, primary container
ast agent logs my-agent --workload agent/messaging
ast agent logs my-agent --workload vectors       # knowledge entry, primary container
ast agent logs my-agent --workload my-agent-knowledge-vectors/app
```

When the resolved workload's container list is known locally (from the deployment detail response), the CLI validates the suffix and returns a list of available containers on miss. Otherwise the suffix is forwarded to the server, which already validates against the pod's actual container list.

The four error strings introduced by PR #1210 — no agent workload, no match, ambiguous match, unknown container — moved to `apps/astro-cli/cmd/messages.go` as `errXxx` functions per the package convention.

## Migration

Replace `--container <name>` with `--workload <workload>/<name>` (or drop `--container` entirely if you were passing `app`, since that's the new default):

| Before | After |
|---|---|
| `ast agent logs my-agent --container app` | `ast agent logs my-agent` |
| `ast agent logs my-agent --container messaging` | `ast agent logs my-agent --workload agent/messaging` |
| `ast agent logs my-agent --workload vectors --container app` | `ast agent logs my-agent --workload vectors` |
