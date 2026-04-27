## Summary

Agent names containing uppercase letters, underscores, or dots were rejected by the Docker daemon during `astro push` with "repository name must be lowercase", and caused log lookups to silently return nothing in local dev where Loki is not configured. This change establishes a single canonical naming policy enforced by both the CLI and the server.

## Design

Agent names must be 4–63 characters, lowercase alphanumeric with hyphens, start with a letter, end with alphanumeric, and not be a reserved platform name (`astro`, `agent`, `model`, `integration`).

**Shared policy (`astro-spec`)**: `packages/astro-spec/naming.go` defines the canonical naming rules for both agent names and variable (secret) names. Both the CLI and the server import this package, so rules are defined once and cannot drift.

- `ValidateName` / `IsValidName`: agent name policy (4–63 chars, lowercase alphanumeric with hyphens, reserved name check).
- `ValidateVarName` / `IsValidVarName`: variable name policy (POSIX env var convention: letters, digits, underscores; must start with a letter or underscore).

**Server (`astro-server`)**: All API boundaries validate names — accepted if valid, rejected with 400 if not:

- `RegisterAgent` (`POST /api/v1/agents/:account/:name/register`): agent name validated after the existing `@org/` prefix check.
- `prepareDeployment`: `source.name` in the submitted spec is validated before the agent lookup.
- `GetDeploymentLogs` / `StreamDeploymentLogs`: the `?workload=` query parameter (K8s Deployment/StatefulSet name, used as a Loki pod-label prefix) is validated before pod resolution.
- `CreateAccountVariable`: variable name validated with `spec.IsValidVarName` before upsert.

**CLI (`astro-cli`)**: `scaffold.ValidateName` is removed; `cmd/blueprint.go`, `cmd/create.go`, and `internal/tui/create/run.go` call `spec.ValidateName` directly. The local `secretNameRe` / `validateSecretName` in `cmd/secrets.go` is replaced with `spec.ValidateVarName`.

Names with uppercase letters, underscores, spaces, fewer than 4 characters, or other invalid characters are rejected with a clear error message.

## Migration

Names that are already valid (lowercase, hyphen-separated, 4+ characters) require no action. Names shorter than 4 characters or containing uppercase letters, underscores, dots, or spaces must be corrected in the spec before pushing.
