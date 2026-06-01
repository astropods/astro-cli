# Faster Go CI: cached golangci-lint, drop redundant vet

## Summary

The `test-go` matrix job for astro-server was spending ~10 minutes per run, but only ~80s was actual `go test` execution. Most wall time went to compiling `golangci-lint@latest` from source on every run and re-analyzing the full module tree without cache. This change targets that overhead.

## Design

**Lint** — replace `go install golangci-lint@latest` + manual `golangci-lint run` with `golangci/golangci-lint-action@v7` (pinned to `v2.12.2`). The action downloads a prebuilt binary and restores the analysis cache between runs. Each matrix app still lints in its own `working-directory`; the shared `.golangci.yaml` at repo root is discovered as today.

**Vet** — remove the standalone `go vet ./...` step. `govet` is already enabled in `.golangci.yaml`, so the separate pass was redundant work on every matrix leg.

**gotestsum** — pin to `v1.13.0` across all jobs in `test.yml` (unit, CLI integration, Postgres integration, K8s integration) instead of `@latest`.

Expected impact: astro-server job drops from ~10 min to ~2–3 min on warm cache; cold-cache lint may still take ~6 min once before the analysis cache kicks in (CI timeout set to 10m to accommodate).

## Migration

No action required.
