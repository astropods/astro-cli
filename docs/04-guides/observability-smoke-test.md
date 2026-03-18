# Observability Smoke Test (Mastra -> Collector -> Galileo -> Astro Server)

Use this checklist to validate the full telemetry path for local development.

## Preconditions

- Astro repo available at `ASTRO_ROOT`
- Agent project created with the Mastra template
- Galileo credentials available locally (`GALILEO_API_KEY`, `GALILEO_PROJECT`)

## One-time setup (shell session)

```bash
export ASTRO_ROOT="/path/to/astro"
export GALILEO_API_KEY="..."
export GALILEO_PROJECT="..."
```

If you are testing branch changes to the CLI, build and use the local binary:

```bash
cd "$ASTRO_ROOT"
go build -o /tmp/ast ./apps/astro-cli
alias ast="/tmp/ast"
```

## Runtime smoke test checklist

Run the following from the agent project directory:

```bash
# 0) Clean start
ast dev stop || true

# 1) Start local stack
ast dev --local

# 2) Verify expected services are present
ast dev logs astro-collector
ast dev logs agent
ast dev logs astro-messaging

# 3) Generate traffic
# Open playground UI and send 3-5 prompts:
#   http://localhost:3000

# 4) Validate hops
# Agent -> collector
ast dev logs agent
# Expect: no OTEL exporter failures

# Collector -> galileo
ast dev logs astro-collector
# Expect: traces processed/exported, no GALILEO auth/export errors
```

## Expected signals per hop

- `agent -> collector`
  - `agent` logs do not show OTLP connection/export failures.
  - `OTEL_EXPORTER_OTLP_ENDPOINT` resolves to `http://astro-collector:4318` in compose mode.

- `collector -> galileo`
  - `astro-collector` logs show trace pipeline activity.
  - No 401/403 responses from Galileo exporter.

- `galileo -> astro-server`
  - Astro server observability endpoints return non-empty payloads for active test runs:
    - `/api/v1/agents/:account/:name/observability/metrics`
    - `/api/v1/agents/:account/:name/observability/summary`
    - `/api/v1/agents/:account/:name/observability/traces`

## Fast failure guide

- `no such service: astro-collector`
  - You are likely using an older `ast` binary. Build from current branch and alias to `/tmp/ast`.

- `ASTRO_ROOT is not set`
  - Required for `ast dev --local`; export `ASTRO_ROOT` before running.

- Empty observability payloads in Astro UI/API
  - Confirm prompts were sent in playground.
  - Check collector logs for Galileo auth or exporter errors.
  - Ensure `GALILEO_API_KEY` and `GALILEO_PROJECT` are set in your shell.
