## Summary

The `ast push` step in the CLI smoke tests fails on preview with a 403 from the registry proxy — the registry is not accessible from GitHub Actions in that environment. Rather than disabling the smoke job entirely, the CLI projects are conditionally excluded from the Playwright run on preview, so auth, marketing-site, blueprint, and secrets tests continue to run.

## Design

`playwright.prod.config.ts` now reads `ASTRO_ENV` at config load time and spreads the CLI-dependent project definitions only when not running against preview:

```ts
const isPreview = process.env.ASTRO_ENV === "preview";

projects: [
  // always run: setup, marketing-site, blueprints, auth, app.secrets
  ...(!isPreview ? [
    // cli, cli-teardown, app.deploy, cli.post-deploy, app.post-deploy
  ] : []),
]
```

The excluded projects form a complete dependency chain (cli → app.deploy → post-deploy/cli.post-deploy), so no dangling dependency references are left behind when they are dropped. The smoke-test CI job remains enabled in `deploy-preview.yml`.

The `exec()` helper in `cli-state.ts` also gains a `retries` option that retries only the individual command rather than restarting the entire serial test group, which is the correct granularity for transient network errors during push.

## Migration

No action required.
