# scripts/

- [local-dev.sh](#local-devsh)
- [e2e.sh](#e2esh)
- [smoke-test.sh](#smoke-testsh)
- [update-submodules.sh](#update-submodulessh)
- [validate-submodules.sh](#validate-submodulessh)
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
- `modules/astro-cli/bin/ast-dev` — local CLI (built pointed at `http://localhost`)

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

## smoke-test.sh

Runs the Playwright smoke test suite against a live environment. Defaults to `ASTRO_ENV=dev` (local dev server at `http://localhost`).

```bash
# Against local dev server (requires local-dev.sh running)
ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... ASTRO_TEST_USERNAME=... moon run tests:smoke

# Against prod
ASTRO_ENV=prod ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... moon run tests:smoke

# With Playwright UI
ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... ASTRO_TEST_USERNAME=... moon run tests:smoke -- --ui
```

Requires `ASTRO_TEST_EMAIL` and `ASTRO_TEST_PASSWORD`. In dev mode, `ASTRO_TEST_USERNAME` is also required (your account handle). The test account must be on the WorkOS CAPTCHA bypass allow list.

## update-submodules.sh

Syncs submodule URLs, checks out the SHAs recorded in the superproject, and falls back to each submodule's remote `HEAD` only when a recorded SHA is missing from its remote (e.g. an unpushed or cross-repo pointer). Prints instructions when working trees diverge from recorded pointers.

```bash
bash scripts/update-submodules.sh
```

## validate-submodules.sh

Offline check that every `.gitmodules` path has a gitlink at the given ref and that no two submodule paths share the same SHA (catches cross-repo mix-ups like committing a `modules/blog` SHA as `modules/website`). Does not fetch submodule remotes — use `update-submodules.sh` locally to verify SHAs exist. Runs in CI on submodule pointer changes.

```bash
bash scripts/validate-submodules.sh        # validate HEAD
bash scripts/validate-submodules.sh main   # validate a specific ref
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

Port-forwards an agent pod's messaging HTTP/SSE port (8090) to localhost. This is the endpoint the web-experience hits, so it's what you want when driving the local web UI against a running agent.

```bash
bash scripts/agent-k8sfwd.sh <agent-name> [host-port]
```

Defaults to `localhost:18090` (avoids the common 8090 collision with locally-published Docker containers). Always runs against the `docker-desktop` kubectl context — override with `AGENT_K8SFWD_CONTEXT=<name>`. Never mutates your active context. Prints the context, namespace, pod, PID, and a `kill` command for cleanup. Kills any matching `kubectl port-forward` on this agent's 8090 before starting a new one.

## merge-svg-pixels.ts

Merges adjacent same-colored rectangles in pixel-art SVGs to eliminate the sub-pixel "screen door" gap artifact that appears when adjacent `<path>` rectangles are rendered at non-integer scales.

```bash
bun scripts/merge-svg-pixels.ts <input.svg> [output.svg]
```
