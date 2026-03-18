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
