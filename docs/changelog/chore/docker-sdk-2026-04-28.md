## Summary

Replaces all remaining raw `exec.Command("docker", ...)` calls in the CLI with the moby/moby and docker/compose SDK equivalents, and centralises Docker client creation into a `newDockerClient()` factory that checks daemon reachability at first use.

## Design

### Replaced calls

| Old | New |
|-----|-----|
| `exec.Command("docker", "tag", local, remote)` | `client.ImageTag(ctx, ImageTagOptions{...})` |
| `exec.Command("docker", "info")` | `client.Ping(ctx, PingOptions{})` |
| `exec.Command("docker", "compose", "-p", name, "ps", ...)` | `newComposeService(false).Ps(ctx, name, PsOptions{All: true})` |

The `docker tag` call was in the local-dev retag path of `runPush` (skip-push + build). The `docker info` and `docker compose ps` calls were in `checkDockerRunning` and `checkComposeHealth` in `dev.go`.

### `newDockerClient()` singleton

Previously, Docker availability was only checked upfront in `ast dev` commands via `checkDockerRunning`. Other commands (`ast build`, `ast push`) called `client.New(client.FromEnv)` directly, so a missing or stopped daemon produced a raw SDK error mid-operation:

```
Pre-pull skipped (oven/bun:1): failed to connect to the docker API at unix:///var/run/docker.sock; ...
```

All `client.New(client.FromEnv)` call sites now use `newDockerClient()` instead. The function uses `sync.Once` to create and `Ping` the client exactly once per process invocation; on failure it checks `/var/run/docker.sock` via `os.Lstat` and returns a styled error:

- Socket absent → "Docker is not installed" with an install link
- Socket present but unreachable → "🐳 Docker is not running" with a start hint

Callers must not close the client — it is a singleton and closing it would break subsequent callers. `cmd.CloseDockerClient()` is exported for `main` to defer at process exit. `checkDockerRunning` in `dev.go` is now a two-line wrapper around `newDockerClient()`.

### Docker install detection

`checkDockerRunning` previously used `exec.LookPath("docker")` to distinguish "not installed" from "not running". The CLI no longer requires the Docker binary in `$PATH` at all; the install check is based solely on socket presence.

## Migration

No user-facing changes. Error messages are unchanged; they now fire earlier and consistently across all commands.
