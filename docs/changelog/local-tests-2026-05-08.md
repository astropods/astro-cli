## Summary

Restructures the smoke test suite for better local dev ergonomics and test reliability. Tests previously lived at the repo root; they are now co-located with the app code and can run against a local dev server or prod.

## Design

**Relocation and config rename.** Tests moved from the repo root to `apps/tests/smoke/`. The Playwright config was renamed `playwright.smoke.config.ts` and updated to use `import.meta.dirname` throughout (the project uses ESM, so `__dirname` is unavailable). `baseURL` uses `||` instead of `??` so an empty `ASTRO_TEST_HOST` falls back to the config default rather than producing an invalid URL.

**Dev mode.** A new `ASTRO_ENV=dev` configuration points the test suite at `http://localhost`. In dev mode the CLI install step is skipped (`test.skip`), the local binary is resolved from `apps/astro-cli/bin/` using `envConfig.cliName`, and the marketing-site and blueprint public-page tests are skipped (those environments don't exist locally). `ASTRO_ENV` is required — the suite throws on startup if it is unset or invalid. The `moon run tests:smoke` wrapper defaults it to `dev` automatically.

**ASTRO_ENV enforcement.** `env.ts` validates that `ASTRO_ENV` is one of `"dev"`, `"preview"`, or `"prod"` at module load time. Running `bunx playwright test` directly without setting `ASTRO_ENV` produces a clear error rather than silently using a broken default.

**Fixtures seam.** All specs import `test` and `expect` from a shared `fixtures.ts` rather than directly from `@playwright/test`, making it easy to wrap the test object later (e.g. for Postman network capture) without touching every spec file.

**Parameterized username.** The test account username is now part of `envConfig` rather than a standalone env var. `prod` and `preview` default to `astro-testbot`; `dev` reads `ASTRO_TEST_USERNAME` from the environment. All specs use `envConfig.username`; no CI env var needed. CLI binary name (`cliName`) also comes from `envConfig` so all three environments use the correct binary.

**Moon task.** `apps/tests/moon.yml` adds a `smoke` task that runs `scripts/smoke-test.sh` via `$MOON_WORKSPACE_ROOT` (resolves to the workspace root regardless of which directory Moon runs from). The task has `cache: false` and `runInCI: false`.

**Smoke-test script.** `scripts/smoke-test.sh` centralises environment setup: validates required env vars, defaults `ASTRO_ENV=dev`, and accepts `--postman` (runs via `postman app test`) and `--ui` (appends `--ui` to the Playwright command) flags.

**Reliability fixes.**
- `app.secrets`: Secrets row assertion scoped to the exact row via case-sensitive regex filter, avoiding collisions with pre-existing lowercase variables.
- `app.deploy`: Deployment slug captured via regex group with `expect(slugMatch).toBeTruthy()` guard instead of `.split("/").pop()`. The slug is also persisted to `CLI_STATE_FILE` for downstream teardown tests.
- `cli.spec`: `installHost` asserted to be non-empty before running `curl | sh`. Install timeout doubled to 120 s. `ast settings update --telemetry off` runs immediately after install.
- `cli.teardown` / `cli.post-deploy`: `astBin` read from `CLI_STATE_FILE` (written during push) rather than reconstructed, so the correct binary path (local dev or sandbox-installed) is used across all downstream tests.
- Unused imports removed from `cli.teardown.ts` and `cli.post-deploy.spec.ts`.

## Migration

- No required env var changes for CI — username defaults are baked into the config.
- `ASTRO_ENV` must now be set explicitly (`dev`, `preview`, or `prod`). Use `moon run tests:smoke` for local dev — it defaults `ASTRO_ENV=dev` automatically. For direct invocation: `ASTRO_ENV=prod ASTRO_TEST_EMAIL=... ASTRO_TEST_PASSWORD=... bunx playwright test --config=apps/tests/smoke/playwright.smoke.config.ts`.
- Use `moon run tests:smoke -- --ui` for the Playwright UI (replaces the old `UI=1` env var).
