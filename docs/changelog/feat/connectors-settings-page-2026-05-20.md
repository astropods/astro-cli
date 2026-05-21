# Connectors settings page

## Summary

GitHub and Slack identity links lived as two separate sections inside the personal Account settings page, with mismatched chrome (Switch vs button to disconnect, different row layouts, different status copy). They didn't read as the same kind of thing, and the Account page bundled identity links with email/timezone/danger-zone — three unrelated concerns on one screen.

This change splits identity connectors out into their own settings page and unifies the per-connector UI.

## Design

**New route `/settings/connectors`** registered alongside `account`, `usage`, `secrets`, etc., with a `Plug` icon nav item between Variables & Secrets and Organizations. `OAuth callback redirects (`?github_connected=…`, `?slack_connected=…`) now return to `/settings/connectors`. Account settings keeps only Account, Preferences, and Danger Zone.

**Shared shell.** `components/settings/ConnectorCard.tsx` exports `ConnectorCard` (header: brand icon, status line, optional meta line, action area) and `ConnectorCardRow` (`<li>` with shared padding/typography/background). Both connectors render through it so spacing, dividers, and status copy patterns match.

**GitHub card** now mirrors the Slack pattern. The OAuth org scope is rendered as one row per granted org (avatar + login) — Reauthorize re-runs the OAuth flow so users can broaden grants, Disconnect opens the existing type-to-confirm dialog. Empty state ("No organizations granted. Use Reauthorize to add access.") covers personal-only scope. Repos list was dropped from this card since it doesn't fit the per-identity row model — repos remain visible per agent.

**Slack card** rows show workspace icon + team name on the left, with `@username` + a copy-to-clipboard button (for `slack_user_id`) and disconnect-X grouped on the right via `ml-auto`. The user ID itself isn't shown — the copy button is the affordance, since users only need the ID to drop into config and rarely need to read it.

**Slack description** clarifies the connector's purpose: it maps the user's Astro identity to their Slack identity so the auth layer can resolve who's calling when requests arrive from Slack. It is not for outbound impersonation.

**Server: `GET /api/v1/accounts/:account/github/orgs`** returns `{ orgs: [{login, avatar_url}] }` from the user's OAuth token. New `Org` type + `ListOrgs` method on `internal/github.Client` (separate from `GetOrgs`, which still returns `[]string` for repo-search filtering). Auth/error semantics match the rest of the account-GitHub handlers: 401 unauthenticated, 422 `github_not_connected` on stale/missing token, 500 otherwise.

## Migration

None. OAuth return-URL changes are transparent — links already in flight at deploy time still land on a valid page (the GitHub/Slack URL params are stripped on first render either way).
