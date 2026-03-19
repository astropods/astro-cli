# Python starter: CLI flag, templates, and local dev support

## Summary

Adds Python agent support to the CLI so `ast create --lang py` scaffolds a working Python agent project and `ast dev` runs it with the same local dev experience as TypeScript agents.

## Design

**`ast create --lang py`** — the `create` command now accepts `--lang py` alongside `--lang ts`. Passing `--lang py` defaults the template to `langchain`; `--lang ts` defaults to `mastra`. Template/language combinations are validated at parse time. Scaffold generation is language-aware: TypeScript-only files (`package.json`, `tsconfig.json`, `agent/index.ts`) are skipped for Python, and Python-only files (`requirements.txt`, `agent/main.py`, `ingestion/*/main.py`) are skipped for TypeScript.

Two new template directories are added:

- `template-py/` — shared Python files: `Dockerfile`, `Dockerfile.ingestion`, `.gitignore`, `.dockerignore`, `README.md`.
- `template-py-langchain/` — LangChain-specific files: `agent/main.py` and `requirements.txt`.

**`ast dev` and `ast dev --local` for Python** — language is detected via `isPython`/`isTypeScript` flags (presence of `requirements.txt`). TypeScript is the default code path; Python is secondary. The two modes behave symmetrically across both languages:

- Without `--local`: the agent runs in a Docker container and installs published packages (npm for TypeScript, PyPI for Python) via the container's dependency file.
- With `--local`: the agent runs as a local process. TypeScript agents symlink `@astropods/*` packages from ASTRO_ROOT and build SDKs. Python agents install editable packages from ASTRO_ROOT via `pip install -e` in dependency order (messaging → adapter-core → adapter-langchain).

`--local-reset` supports both languages: TypeScript removes symlinks and restores via `bun install`; Python uninstalls editable packages and restores from `requirements.txt`.

Three Python helpers parallel the TypeScript equivalents:
- `localAstroPythonPackages` — ordered slice of packages (mirrors `localAstroPackages`)
- `installLocalPythonPackages()` — editable pip installs from ASTRO_ROOT (mirrors `linkLocalPackages`)
- `uninstallLocalPythonPackages()` — removes editable installs and restores from PyPI (mirrors `unlinkLocalPackages`)

**Ingestion container dependencies** — the Python `Dockerfile.ingestion` template uses a multi-stage build that installs dependencies from `ingestion/<type>/requirements.txt`, matching the TypeScript ingestion Dockerfile. The scaffold generates an `ingestion/<type>/requirements.txt` alongside each `main.py`.

**Agent startup logs** — `serve()` in `astropods-adapter-core` calls `logging.basicConfig()` with `force=True` before starting the bridge so the startup log lines ("Starting...", "Connected to messaging service", "ready and listening") are visible in `ast dev logs`. `PYTHONUNBUFFERED=1` is set in the Python agent Dockerfile to prevent stdout buffering in Docker.

## Migration

No changes required for existing TypeScript agents or deployments. This is the first release of Python agent support in the CLI.

## Testing

Integration tests for the Python scaffold live in `apps/astro-cli/e2e/` behind the `integration` build tag. They generate a real project with `GenerateFiles` and run `docker build` to confirm the Dockerfiles are valid. Run locally with:

```bash
moon run astro-cli:e2e
# or directly:
go test -tags integration -v ./e2e/...
```

The `test-cli-integration` GitHub Actions job runs these automatically on every PR.

## Test plan

### Scaffold

- [ ] `ast create my-py-agent --lang py` generates a project with `agent/main.py`, `requirements.txt`, `Dockerfile`, `astropods.yml`, `.gitignore`, `.dockerignore`, `README.md` — no `package.json`, `tsconfig.json`, or `agent/index.ts`
- [ ] `ast create my-py-agent --lang py --template langchain` produces the same result (explicit template flag)
- [ ] `ast create my-ts-agent --lang ts` still generates a TypeScript project unchanged
- [ ] `ast create my-agent --lang py --template mastra` returns an error: `template mastra requires --lang ts`
- [ ] `ast create my-agent --lang ts --template langchain` returns an error: `template langchain requires --lang py`
- [ ] `ast create my-agent --lang ruby` returns an error: `unsupported language`
- [ ] Creating with `--lang py` and ingestion types (e.g. `webhook`, `startup`) generates `ingestion/<type>/main.py`, `ingestion/<type>/requirements.txt`, and `ingestion/<type>/Dockerfile` for each type
- [ ] The generated `README.md` project structure tree includes `requirements.txt` under each ingestion type folder

### `ast dev` (Docker mode)

- [ ] `ast dev` in a Python agent directory starts the agent container; `ast dev logs` shows "Starting...", "Connected to messaging service", and "ready and listening"
- [ ] `ast dev` in a TypeScript agent directory is unaffected

### `ast dev --local`

- [ ] `ast dev --local` in a Python agent directory prints `Using local Python packages from <ASTRO_ROOT>` and pip-installs `astropods-messaging`, `astropods-adapter-core`, and `astropods-adapter-langchain` as editable packages in that order
- [ ] After `--local`, the agent starts with `python -m agent.main` and logs are visible via `ast dev logs`
- [ ] `ast dev --local-reset` in a Python agent directory uninstalls the editable packages and restores from `requirements.txt`
- [ ] `ast dev --local` in a TypeScript agent directory still symlinks `@astropods/*` packages as before

### Ingestion containers

- [ ] Building the ingestion Dockerfile for a Python agent succeeds (`docker build -f ingestion/<type>/Dockerfile .`)
- [ ] The ingestion container installs dependencies from `ingestion/<type>/requirements.txt` and runs `python ingestion/<type>/main.py`
- [ ] Adding packages to `ingestion/<type>/requirements.txt` and rebuilding picks them up correctly

### Startup logs

- [ ] `ast dev logs` for a Python agent shows the adapter startup sequence without needing to set any extra env vars
- [ ] Restarting the agent (`ast dev restart`) shows the startup logs again
