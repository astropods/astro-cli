# Fix deploy form Slack variable handling and add client E2E coverage

## Summary

Some deployment configurations could fail with `SLACK_BOT_TOKEN.value has no value` even when users had entered a token in the UI, and the client had no browser-level end-to-end coverage to prevent regressions in template-to-form-to-payload behavior. This change hardens deploy-form value resolution, aligns submission with template-defined adapter fields, adds first-class client E2E tests with a deterministic mock backend, and makes CI execution resilient to workspace-package build ordering.

## Design

The deploy form now treats user-entered values as a unified source of truth across template variables and adapter credential fields, instead of assuming each key exists in only one state bucket.

- **Unified value resolution**: validation and submit-time payload construction use merged effective values, so overlapping keys (for example, a Slack token targeted to both `agent` and `interface.slack`) cannot appear filled in UI but empty at submit.
- **Template-contract submission**: adapter variable injection during submit now follows template-derived adapter field definitions (including optionality) rather than a separate hardcoded list, preventing hidden extra required fields from being injected.
- **Bulk import parity**: bulk import matching writes overlapping keys into all relevant form buckets, and import visibility is based on all importable keys (required/optional variables plus adapter fields), so import remains available for adapter-heavy templates.
- **Client E2E harness**: Playwright now runs against a dedicated mock backend and app server pair, so both SSR loaders and client fetches see the same controlled responses. Coverage now includes app-token-only Slack templates, full Slack config with reactions, optional reactions omission, overlapping Slack token targets, variable import flows, configure-page `Save & Redeploy` flows, and explicit server-rejection behavior (validation error rendering without redirect) for deploy failures.
- **Deterministic CI orchestration**: The `astro-client:e2e` Moon task now declares build dependencies on shared workspace libraries used at runtime by the Vite build pipeline, and the GitHub workflow executes the Moon target directly. This removes ad hoc prebuild ordering from CI and prevents package-entry resolution failures when client E2E runs in a clean environment.

## Migration

No migration is required. Existing deployments continue to work, while configure/install flows become more robust when template variables overlap adapter targets or when users import variables from `.env`/JSON text files.
