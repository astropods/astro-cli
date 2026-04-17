# GitHub Import Path & Onboarding Tweaks

## Summary

Adds a GitHub import path to the new blueprint wizard, lets users connect an existing repo instead of starting from scratch, and completes the feature with repo-locking (already-linked repos are disabled in the picker), automatic disconnect on archive, and comprehensive test coverage.

## Design

### Multi-step wizard with GitHub import path

A `source` step is inserted between identity setup and publish. Users choose between "Set up locally" (existing CLI flow, unchanged) or "Set up with GitHub". Selecting GitHub triggers account-level OAuth; on return the wizard restores its state from `sessionStorage` and drops the user back into repo selection without losing name/org/visibility choices.

**Account-level GitHub OAuth** (`POST /accounts/:account/github/connect`, `GET /accounts/:account/github/callback`): Previously the only OAuth entry point was blueprint-specific. The new endpoints are blueprint-agnostic — they issue or reuse a Pipes token and accept a `redirect_to` field so the callback can return the browser to any frontend URL (e.g. `/new/custom?github_connected=true`). Repo listing is exposed at `GET /accounts/:account/github/repos`.

**Admin-only repo filter**: `ListRepos` filters to repos where `permissions.admin === true`. Non-admin repos can't have webhooks installed, so showing them was misleading.

**Wizard publish wires up the link**: After blueprint creation, if the user chose the import path, `githubLink` is called in the same publish batch. Failures are swallowed so a webhook error doesn't strand the wizard. The link can be recovered from the detail page sidebar.

### Repo picker: disable already-linked repos

The repo dropdown fetches the account's existing GitHub connections (`GET /api/v1/accounts/:account/github/connections`) and disables any repo already linked to another blueprint, showing which blueprint it belongs to. This requires a new backend endpoint backed by `githubconnection.Store.ListByAccount` and a `useGitHubAccountConnections` TanStack Query hook.

### Disconnect GitHub repo on archive

When a blueprint is archived, the server fires a best-effort goroutine to delete the GitHub webhook and remove the `github_connections` row, freeing the repo to be reused immediately. The goroutine is nil-guarded so tests passing `nil` for `ghStore` are unaffected.

The `useArchiveBlueprint` mutation invalidates both `githubKeys.accountConnections(account)` and `githubKeys.status(account, name)` in `onSuccess`, so the wizard's repo picker updates without a page refresh.

### Draft detail page

`BlueprintDetailContent` receives `githubRepoName` and `visibility`. When a GitHub repo is connected it switches the "Finish setup" panel to a two-step GitHub flow (add `astropods.yml`, commit & push) instead of the three-step CLI flow. The panel header icon swaps from terminal to the GitHub mark.

`ConnectedRepoView` shows an amber pulsing dot with "Waiting for astropods.yml" when no builds exist. Once a webhook fires the existing build rows render as before.

The review step no longer auto-navigates when `sourcePath === "import"` — the user lands on the draft page to see the GitHub setup instructions.

**Scan-before-link**: The publish flow scans for `astropods.yml` before installing the webhook, so the wizard knows whether to trigger an immediate build. Scan errors are treated as not-found.

**Upsert-before-webhook**: `GitHubLink` saves the connection row before attempting webhook creation. Webhook creation is best-effort — if it fails the connection persists so subsequent rebuild calls can succeed.

### Tests

- **Go**: Nil-guard for the disconnect goroutine; `visibility_test.go` updated for the new `ArchiveAgent` signature; `client_test.go` adds `Permissions.Admin: true` so mock repos pass the admin filter.
- **E2E — existing tests**: `blueprint-creation.spec.ts` and `onboarding-flow.spec.ts` updated to navigate the multi-step wizard (identity → Continue → source → Create blueprint).
- **E2E — new tests**: `github-onboarding.spec.ts` covers 4 flows: local setup, GitHub import, linked-repo disabled in picker, and archive-releases-repo.
- **Mock backend**: GitHub endpoints added (`connect`, `repos`, `connections`, `scan`, `link`, `unlink`, `status`); archive handler clears connections in mock state.

## Migration

No migration required. Existing blueprints and the local CLI path are unaffected.

---

# Code Review Pass (2026-04-16)

## Summary

Bug fixes and cleanup on top of the onboarding feature: security fixes, a React hooks correctness fix, dead code removal, and new unit test coverage.

## Design

**Security fixes (`handlers/github.go`)**: `redirect_to` is now validated to reject absolute URLs and protocol-relative paths (open redirect prevention). The value is also properly percent-encoded when embedded in the callback URL.

**gin.Context data race (`handlers/agents.go`)**: The archive goroutine was reading from `gin.Context` after the handler returned. `GetSession` is now called before the goroutine launches and the result is captured by closure.

**Nil-slice JSON (`internal/github/client.go`)**: `ListRepos` now initialises the result slice with `make([]Repo, 0, ...)` so a zero-match result serialises as `[]` not `null`.

**Variable shadowing (`internal/githubbuild/builder.go`)**: Local variable `ecrRepoName` shadowed the package-level function of the same name. Replaced with a direct call to the function.

**React Rules of Hooks (`pages/BlueprintDetail.tsx`)**: Hooks were called after conditional early returns (`isError`, `!blueprint`). Fixed by splitting into `BlueprintDetail` (outer shell that handles loading/error) and `BlueprintDetailInner` (private component that owns all hooks and interactive state).

**Shared `GitHubIcon` component**: The GitHub SVG was duplicated verbatim in `BlueprintDetailContent.tsx` and `NewBlueprint.tsx`. Extracted to `components/ui/svgs/githubIcon.tsx` and both files import the shared component.

**`statusColor` hoisted to module scope** (`GitHubConnectionPanel.tsx`): The function was re-created inside `BuildLogsDialog` on every render. Moved to a pure module-level function.

**`parseAgentMD` moved to `blueprint-utils.ts`**: The AGENT.md parser was an unexported local in `BlueprintDetail.tsx`. Moved and exported so it can be tested in isolation.

**`knowledgeKeys` placement (`keys.ts`)**: Moved from between `githubKeys` and `deploymentKeys` to the end of the file for logical grouping.

## Tests added

- `src/api/queries/github.test.ts` — `useGitHubStatus`, `useGitHubRebuild`, `useGitHubAccountConnections` hook coverage
- `src/lib/blueprint-utils.test.ts` — `parseAgentMD` (9 cases), `getEffectiveCard` (4 cases), `getBlueprintReadme` (3 cases), `getBlueprintDescription` (2 cases)
- `internal/githubbuild/builder_test.go` — `ecrRepoName` (6 cases) and `EnsureRepository` (5 cases) added
