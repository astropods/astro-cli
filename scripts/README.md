# scripts/

- [local-dev.sh](#local-devsh)
- [e2e.sh](#e2esh)
- [update-submodules.sh](#update-submodulessh)
- [release.ts](#releasets)
- [agent-k8sfwd.sh](#agent-k8sfwdsh)
- [merge-svg-pixels.ts](#merge-svg-pixelsts)

## local-dev.sh

Starts astro-server and astro-client behind Traefik, and builds the `ast-dev` CLI. Ctrl-C stops everything and tears down Docker.

```bash
bash scripts/local-dev.sh
```

- `http://localhost` — astro-client
- `http://localhost/api` — astro-server
- `http://localhost:8090` — Traefik dashboard
- `apps/astro-cli/bin/ast-dev` — local CLI (built pointed at `http://localhost`)

Requires Docker and Moon to be running.

## e2e.sh

Manages the e2e test infrastructure (kind cluster + vcluster + Postgres) and runs the astro-server e2e test suite.

```bash
./scripts/e2e.sh              # spin up infra and run all e2e tests
./scripts/e2e.sh setup        # spin up infra only
./scripts/e2e.sh teardown     # destroy cluster and Postgres container
./scripts/e2e.sh integration  # Postgres-only, run -tags integration tests
```

Requires: `docker`, `kubectl`, `go`. Installs `kind` and `vcluster` via Homebrew if missing.

## update-submodules.sh

Checks out `main` and pulls the latest changes for every git submodule, then prints instructions for committing the updated pointers to the parent repo.

```bash
bash scripts/update-submodules.sh
```

## release.ts

Cuts a release: bumps the version, generates release notes from changelogs, tags the commit, and optionally pushes.

```bash
bun scripts/release.ts                 # auto-detect bump from commits
bun scripts/release.ts --bump minor    # force a bump type (patch | minor | major)
bun scripts/release.ts --dry-run       # preview without writing anything
bun scripts/release.ts --yes           # skip confirmation prompt
```

## agent-k8sfwd.sh

Port-forwards an agent pod's gRPC observability port (8090) to localhost. Useful for inspecting a running agent deployment with local tooling.

```bash
bash scripts/agent-k8sfwd.sh <agent-name>
```

Kills any existing `kubectl port-forward` processes before establishing the new one.

## merge-svg-pixels.ts

Merges adjacent same-colored rectangles in pixel-art SVGs to eliminate the sub-pixel "screen door" gap artifact that appears when adjacent `<path>` rectangles are rendered at non-integer scales.

```bash
bun scripts/merge-svg-pixels.ts <input.svg> [output.svg]
```
