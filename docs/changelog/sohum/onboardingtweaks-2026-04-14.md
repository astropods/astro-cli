# GitHub Import Path & Onboarding Tweaks

## Summary

Adds a GitHub import path to the new blueprint wizard, letting users connect an existing repo instead of starting from scratch. Complements this with matching changes to the draft detail page so the setup instructions adapt to the chosen path, and adds an account-level GitHub OAuth flow so the wizard can initiate OAuth without being tied to a specific blueprint.

## Design

**New wizard step — "Starting point"**: A `source` step is inserted between identity setup and publish. Users choose between "Set up locally" (existing CLI flow, unchanged) or "Import from GitHub". Selecting import triggers account-level GitHub OAuth; on return the wizard restores its state from `sessionStorage` and drops the user back into repo selection without losing their name/org/visibility choices.

**Account-level GitHub OAuth** (`POST /accounts/:account/github/connect`, `GET /accounts/:account/github/callback`): Previously the only OAuth entry point was blueprint-specific (`/agents/:account/:name/github/connect`). The new endpoints are blueprint-agnostic — they issue or reuse a Pipes token and accept a `redirect_to` field so the callback can return the browser to any frontend URL (e.g. `/new/custom?github_connected=true`). Repo listing is also exposed at `GET /accounts/:account/github/repos`.

**Admin-only repo filter**: `ListRepos` on the GitHub client now filters to repos where `permissions.admin === true`. Webhook installation requires admin access, so non-admin repos were always going to fail at link time — showing them was misleading.

**Wizard publish wires up the link**: After blueprint creation, if the user chose the import path and selected a repo, `githubLink` is called in the same publish batch. Failures are swallowed (`.catch(() => {})`) so a webhook error doesn't strand the wizard in "initializing". The link can be recovered from the detail page sidebar.

**Draft detail page adapts to path**: `BlueprintDetailContent` now receives `githubRepoName` and `visibility`. When a GitHub repo is connected it switches the "Finish setup" panel to a two-step GitHub flow (drop in `astropods.yml`, commit & push) instead of the three-step CLI flow. The panel header icon also swaps from terminal to the GitHub mark.

**GitHub sidebar waiting state**: `ConnectedRepoView` now shows an amber pulsing dot with "Waiting for astropods.yml" when `builds.length === 0`, replacing the previous static text. Once a webhook fires and builds appear, the existing build rows render as before.

**Auto-advance after publish** (import path): The review step no longer auto-navigates to the blueprint detail page when `sourcePath === "import"` — the user needs to land on the draft page to see the GitHub setup instructions, not skip past it.

## Migration

No migration required. Existing blueprints and the local CLI path are unaffected.
