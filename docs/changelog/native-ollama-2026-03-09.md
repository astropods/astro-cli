# Native Ollama & CLI Dev Tooling

## Summary

Ollama models in dev mode now run on the host instead of inside Docker. Docker-based Ollama can't access the GPU (especially on macOS where nvidia passthrough is impossible), making local model inference painfully slow or non-functional. This change detects the user's native Ollama installation, pulls models directly, and points the agent container at the host via `host.docker.internal`.

Separately, the dev build of the CLI is renamed from `ast` to `ast-dev` so it no longer conflicts with the production binary.

## Design

### Native Ollama in dev mode

When `ast dev` starts and the spec contains Ollama models:

1. **Server detection** — hits `GET /api/tags` on `localhost:11434`. If unreachable, checks whether the `ollama` binary exists to show the right error: install instructions (brew / desktop app / curl script) or start instructions (`ollama serve` / `brew services start ollama`).

2. **Model availability** — queries the Ollama API to list local models. Already-pulled models are skipped.

3. **Resource check** — before pulling, estimates model RAM from the parameter count in the tag (e.g. `70b` -> ~43GB). If the estimate exceeds system RAM (read via `sysctl` on macOS, `/proc/meminfo` on Linux), prompts for confirmation.

4. **Pull via API** — uses `POST /api/pull` with streaming JSON to show download progress inline, instead of shelling out to `ollama pull`.

5. **Compose generation** — `BuildProject` accepts a `NativeOllama` option that skips creating the Ollama Docker service, adds `extra_hosts: host.docker.internal: host-gateway` to the agent container, and sets `OLLAMA_HOST/URL/BASE_URL` to `host.docker.internal:11434`.

### CLI binary rename

The moon build tasks now produce distinct binaries per environment:

| Task | Binary | BinaryName (ldflags) | Config dir |
|------|--------|---------------------|------------|
| `build` / `link` | `ast-dev` | `ast-dev` | `~/.ast-dev/` |
| `build-preview` / `link-preview` | `ast-preview` | `ast-preview` | `~/.ast-preview/` |
| Production (CI) | `ast` | `ast` (default) | `~/.ast/` |

`BinaryName` is injected via ldflags and flows through `auth.ConfigDir` to isolate credentials, project configs, telemetry state, and daemon files. All user-facing text (error messages, help examples, cobra descriptions) uses `binaryName` dynamically instead of hardcoding `ast`.

## Migration

- **Existing `ast` symlink**: Run `moon run astro-cli:unlink` with the old code first (or `rm ~/go/bin/ast`), then `moon run astro-cli:link` to get the new `ast-dev` binary.
- **Config migration**: `ast-dev` uses `~/.ast-dev/` — run `ast-dev login` and `ast-dev configure` to set up credentials in the new location.
- **Ollama**: Install Ollama natively if you haven't already. The containerized fallback has been removed.
