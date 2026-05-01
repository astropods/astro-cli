---
## Summary

Production smoke tests existed only in the marketing website submodule and covered a narrow set of unauthenticated pages. This moves the full test suite to the root monorepo and expands it to cover the authenticated app, the CLI end-to-end, and the complete deploy-and-chat flow against live production.

## Design

Tests are organized as Playwright projects with explicit dependency chains so failures fail fast and teardown always runs:

```
setup → auth → cli ──────────────── cli-teardown (archive + rm sandbox)
                 ↘ app.deploy ──────────────────┘
                      ↘ cli.post-deploy   (ast agent list)
                      ↘ app.post-deploy   (wait for Active)
                           ↘ app.chat     (send message, verify echo)
marketing-site             (parallel, no auth)
app.secrets                (parallel with cli, depends on auth)
```

### Projects

| Project | File | What it does |
|---|---|---|
| `setup` | `auth.setup.ts` | Logs in once, saves `storageState` |
| `marketing-site` | `public.spec.ts` | Public marketing pages — prod only |
| `blueprints` | `public.blueprint.spec.ts` | Blueprint detail page, deploy redirect — all envs |
| `auth` | `auth.spec.ts` | Authenticated app loads, user menu visible |
| `cli` | `cli.spec.ts` | Installs `ast` into sandboxed `HOME`, device-flow login, `account list`, `bp list` precondition, `push` |
| `cli-teardown` | `cli.teardown.ts` | Archives `hello-astro` blueprint, deletes sandbox |
| `app.deploy` | `app.deploy.spec.ts` | Blueprint detail page, deploy flow, captures deployment slug |
| `cli.post-deploy` | `cli.post-deploy.spec.ts` | `ast agent list` confirms deployment slug is present |
| `app.post-deploy` | `app.post-deploy.spec.ts` | Polls `/agents` until Hello Astro shows Active (up to 14 min) |
| `app.chat` | `app.chat.spec.ts` | Opens chat popup, sends message, asserts echo response |
| `app.secrets` | `app.secrets.spec.ts` | Creates variables/secrets, verifies auto-fill on deploy page |

### Key design decisions

**CLI sandboxing** — The install script hardcodes `~/.ast/bin`, so `HOME` is shadowed with `mkdtempSync` for the duration of the CLI tests. `CLI_STATE_FILE` (`.playwright-cli-state.json`) carries `fakeHome`, `pushSucceeded`, and `deploymentSlug` across project boundaries.

**Teardown ordering** — `cli-teardown` is registered as the Playwright `teardown` of the `cli` project. Playwright guarantees it runs only after all projects that depend on `cli` (including `app.deploy`) have finished, so the blueprint is archived last.

**Device-flow login** — `ast login` prints a device URL to stdout/stderr. The test captures it with a regex, navigates the browser to the confirmation page using the saved auth session, and clicks Confirm.

**Deployment slug** — After clicking deploy, the browser lands on `/astro-testbot/agents/{slug}`. The slug is extracted from the URL and written into `CLI_STATE_FILE` so `cli.post-deploy` can match it exactly in `ast agent list`, avoiding false positives from previous deployments.

**Chat popup** — The Chat button calls `window.open()`. The test uses `page.waitForEvent("popup")` (not `context.waitForEvent("page")`) to capture it — the latter resolves with any new page in the context and can race with unrelated pages.

**Running subsets locally** — Use `--no-deps` to skip upstream projects:
```bash
bunx playwright test --config=playwright.prod.config.ts --project=app.chat --no-deps
```

### Environment flags

| Variable | Values | Purpose |
|---|---|---|
| `ASTRO_TEST_HOST` | `https://astropods.com`, `https://astropod.ai` | Base URL for all tests |
| `ASTRO_ENV` | `prod`, `preview` | Gates `marketing-site` (prod only) and selects CLI binary (`ast` vs `ast-preview`) |
| `ASTRO_TEST_EMAIL` | — | Test account email (WorkOS captcha bypass required) |
| `ASTRO_TEST_PASSWORD` | — | Test account password |

### GitHub Actions

`.github/workflows/smoke-test.yml` ("Smoke tests") runs the full suite hourly against `https://astropods.com` (`ASTRO_ENV=prod`). When triggered manually via `workflow_dispatch`, both `astro_test_host` and `astro_env` can be overridden.

The suite also runs automatically after every deploy to preview (`deploy-preview.yml`, `ASTRO_ENV=preview`) and after every prod deploy (`deploy-prod.yml`, `ASTRO_ENV=prod`).

**Credentials** — `ASTRO_TEST_EMAIL` and `ASTRO_TEST_PASSWORD` are stored in the `smoke-tests` GitHub Environment, shared across prod and preview runs. The test account must be on the WorkOS captcha bypass allow list — add it via WorkOS Dashboard → Radar → Configuration → Users (under Custom lists). Without this, login will be blocked by a CAPTCHA and the entire authenticated test tree will be skipped.

**Slack alerts** — On any failure (excluding PR runs), the workflow posts to the channel identified by the `SLACK_ASTRO_OPS_CHANNEL_ID` repository variable using the `SLACK_BOT_TOKEN` secret. The message title says "Production" or "Preview" based on the `astro_env` input.

## Migration

No action required. Tests run automatically on the cron schedule. To run locally, add to `.env.local`:

```
ASTRO_TEST_EMAIL=your@email.com
ASTRO_TEST_PASSWORD=yourpassword
ASTRO_TEST_HOST=https://astropods.com
ASTRO_ENV=prod
```

Then:
```bash
bunx playwright test --config=playwright.prod.config.ts
```
