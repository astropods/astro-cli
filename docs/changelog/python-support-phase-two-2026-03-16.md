# Python starter: CLI flag, templates, and local dev support

## Summary

Phase 1 added the Python adapter packages. This phase wires them into the CLI so `ast create --lang py` scaffolds a working Python agent project and `astro dev --local` runs it against local package sources.

## Design

**`ast create --lang py`** — the `create` command now accepts `--lang py` alongside `--lang ts`. Passing `--lang py` defaults the template to `langchain`; `--lang ts` defaults to `mastra`. Template/language combinations are validated at parse time (`langchain` requires `py`, `mastra` requires `ts`). Scaffold generation is now language-aware: TypeScript-only files (`package.json`, `tsconfig.json`, `agent/index.ts`) are skipped for Python, and Python-only files (`requirements.txt`, `agent/main.py`, `ingestion/*/main.py`) are skipped for TypeScript.

Two new template directories are added:

- `template-py/` — shared Python files: `Dockerfile`, `Dockerfile.ingestion`, `.gitignore`, `.dockerignore`, `README.md`. The ingestion `Dockerfile` does not copy `requirements.txt` since ingestion containers use only stdlib and have no dependency on the agent adapter packages.
- `template-py-langchain/` — LangChain-specific files: `agent/main.py` and `requirements.txt`.

**`astro dev --local` for Python** — `runLocalAgent` now detects Python agents via the presence of `requirements.txt` and installs local packages via `pip install -e` in dependency order (messaging → adapter-core → adapter-langchain) before starting the agent. The default start command is `python -m agent.main`, so `dev.command` does not need to be set in `astropods.yml` for Python agents.

## Migration

This is the first release of Python support in the CLI. No changes to existing TypeScript agents or deployments.
