## Summary

Fixes two failures in the smoke test suite when running against the preview environment, and reduces noise in debugging by logging every CLI command before it runs.

## Design

**Preview login fix**

The auth setup and device-flow fallback login both used `/continue|sign in/i` to click the submit button. The preview login page renders a second "Sign in with a passkey" button that also matches, causing Playwright's strict-mode locator to throw. The regex is now anchored — `/^(continue|sign in)$/i` — so only the primary submit button matches.

**`exec` helper**

An `exec` wrapper in `cli-state.ts` logs `$ <cmd>` then calls `execSync`, replacing the repeated `console.log` + `execSync` pattern across `cli.spec.ts`, `cli.teardown.ts`, and `cli.post-deploy.spec.ts`.

## Migration

No changes required.
