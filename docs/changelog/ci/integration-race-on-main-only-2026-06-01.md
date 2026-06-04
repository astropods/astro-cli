# Skip -race on PR integration and K8s e2e jobs

## Summary

Postgres and K8s integration jobs each spent most of their wall time in `go test -race` on 2-core runners (~4–7 min per job). Unit tests already skip race on PRs; integration jobs did not, making PR feedback unnecessarily slow.

## Design

Mirror the existing `test-go` matrix pattern in `.github/workflows/test.yml`:

- **PRs:** `gotestsum -- -tags integration ./e2e/...` and `gotestsum -- -tags k8s -timeout 5m ./e2e/...`
- **main push:** same commands with `-race` appended

Race detection stays on the merge path where it matters; PRs get faster signal on the same test coverage.

Expected savings per PR: ~3–4 min on K8s integration, ~2–3 min on Postgres integration.

## Migration

No action required.
