---
## Summary

Preview smoke tests were failing due to hardcoded prod-only values scattered across the test suite: the WorkOS login domain, CLI binary name, agent card count, deploy count expectations, author display name, and the app base URL all differ between prod and preview.

## Design

A new `tests/smoke/env.ts` module exports a single `envConfig` object selected at startup based on `ASTRO_ENV`. All environment-specific values live in two named config blocks — `prod` and `preview` — so the difference between environments is visible in one place and tests contain no conditional logic.

```typescript
const prod: SmokeEnvConfig = {
  loginUrlPattern: /login\.astropods\.com/,
  cliName: "ast",
  appBaseUrl: "https://astropods.com",
  minExploreCards: 7,
  minWeatherPoetDeploys: 1,
  authorDisplayName: /Rodric Rabbah/,
  ...
};

const preview: SmokeEnvConfig = {
  loginUrlPattern: /authkit\.app/,
  cliName: "ast-preview",
  appBaseUrl: "https://astropod.ai",
  minExploreCards: 5,
  minWeatherPoetDeploys: 1,
  authorDisplayName: /Rodric Rabbah/,
  ...
};
```

Every test file that previously inlined `process.env.ASTRO_ENV === "preview" ? ... : ...` or hardcoded `login.astropods.com` / `astropods.com` now imports `envConfig` and reads the appropriate field.

The `smoke-test.yml` workflow also gains a `pull_request` trigger scoped to `tests/smoke/**` and `playwright.prod.config.ts`, so the suite runs automatically on PRs that touch the tests. Slack failure alerts are suppressed on PR runs.

## Migration

No action required. `ASTRO_ENV=preview` runs will now resolve to the preview config block automatically.
