# GitHub Import Path & Server-Side Draft Card

## Summary

Adds a GitHub import path to the new blueprint wizard and moves pre-build agent metadata handling to the server. Previously, the client parsed AGENT.md itself and stored results in sessionStorage. Now the server parses it at GitHub link time, stores it as `draft_card_json` on the agent row, and the client reads it through the normal `blueprint.draft_card` field — no sessionStorage, no client-side markdown parsing.

## Design

### Wizard: GitHub import path

A **source** step is added between identity and publish. Users choose "Set up locally" (existing CLI flow, unchanged) or "Set up with GitHub".

GitHub triggers account-level OAuth (`POST /accounts/:account/github/connect` + callback). These endpoints are blueprint-agnostic and accept a `redirect_to` field so the callback can restore the user to the wizard. On return, wizard state (name, org, visibility) is rehydrated from sessionStorage.

The repo picker calls `GET /accounts/:account/github/repos` (admin-only repos — non-admin repos can't receive webhooks) and disables repos already linked to other blueprints via `GET /accounts/:account/github/connections`.

On publish: scan for `astropods.yml` → save connection → install webhook (best-effort) → trigger immediate build if yml found. Users land on the draft detail page instead of auto-navigating away.

### Draft detail page

When a GitHub repo is connected, the "Finish setup" panel switches from the CLI flow to a two-step GitHub flow (add `astropods.yml`, commit & push). An amber pulsing dot indicates waiting for the first build.

When the page detects a draft → published transition on the GitHub path, a full-screen `BuildSuccessOverlay` with confetti fires exactly once, then reveals the blueprint.

`BlueprintDetail` is split into an outer shell (loading/error states) and a private `BlueprintDetailInner` (all hooks) to satisfy React Rules of Hooks.

### Disconnect on archive

Archiving fires a best-effort goroutine to delete the webhook and remove the connection row. The archive mutation invalidates `githubKeys.accountConnections` and `githubKeys.status` so the repo picker updates without a refresh.

### Server-side draft card

`GitHubLink` now fetches AGENT.md after saving the connection, parses it with `spec.ParseAgentCard`, and stores the result in a new `draft_card_json jsonb` column. `AgentResponse` exposes it as `draft_card`. The scan endpoint is simplified to `{ found: bool }`.

`getEffectiveCard` already prefers a published version's `agent_card` over `draft_card`, so the takeover after a successful build is automatic.

**Migration required before deploying:**
```sql
ALTER TABLE agents ADD COLUMN IF NOT EXISTS draft_card_json jsonb NOT NULL DEFAULT 'null';
```
Existing rows back-fill instantly; no lock or table rewrite.

### Security

`redirect_to` is validated to reject absolute URLs and protocol-relative paths. The archive goroutine's `gin.Context` reads are captured before the handler returns.

## Migration

**Ops**: Run the schema migration above in each environment before deploying the server image.

**Users**: No action required. Existing blueprints and the CLI path are unchanged.
