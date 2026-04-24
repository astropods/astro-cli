## Summary

Adds a new `ast agent` command group (aliased `ast agents`) for managing live deployments from the CLI. All commands operate against running deployment state — status, logs, lifecycle controls — not the blueprint registry.

## Design

### Command surface

| Command | Description |
|---|---|
| `ast agent list` | Tabular view: build, status (green/red), blueprint, ID, display name |
| `ast agent get <name>` | Detail view: status, build, deployed timestamp, namespace, ID, components |
| `ast agent pause <name>` | Scale to zero; `--id` bypasses name lookup |
| `ast agent resume <name>` | Wake a paused deployment; `--id` bypasses name lookup |
| `ast agent delete <name>` | Remove a deployment; prompts interactively unless `--confirm <name>` is given |
| `ast agent history <name>` | All revisions: date, build, revision number, status |
| `ast agent restart <name>` | Restart a component's pod; `--component agent` (required); `--id` bypasses name lookup |
| `ast agent logs <name>` | Fetch last 15 minutes of logs; `--tail` switches to SSE streaming; `--container app\|messaging` (required); `--id` bypasses name lookup |

### Name resolution

`findDeploymentByName` resolves a name to a deployment ID by matching exclusively on `DisplayName`. The `--id` flag on `pause`, `resume`, `restart`, and `logs` skips the lookup when the deployment ID is already known.

### Infrastructure

`resolveDeploymentID` centralises the `--id`-or-name-lookup pattern used by the four commands above. `apiStream` is added to `utils.go` for SSE streaming responses (used by `logs --tail`). `apiCall` now prints the request method, URL, and response status code to stderr when `--verbose` is set.

## Migration

No action required. These are new commands with no changes to existing command behaviour.
