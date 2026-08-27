**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-27

# Account and org settings UI

This covers the non-billing, non-secrets settings surface under
`/settings/*` (personal account) and `/settings/org/:orgSlug/*`
(organization): profile/avatar/username management, org membership and
invites, third-party connectors, the audit log viewer, and the experiments
page. It does not cover:

- **Billing** (`*Billing*`, `PayAsYouGoCard.tsx`, `ManageLimitsDialog.tsx`,
  `PaymentMethod.tsx`, `RemovePaymentMethodDialog.tsx`, and `UsageView.tsx`
  even though it lives in this directory: it's built entirely on
  `useBillingUsage`/`useBillingSpend`/`billing-provider` and renders spend,
  invoices, and payment method). See the Billing row in
  [`docs/README.md`](../README.md).
- **Variables & Secrets** (`*Secrets*`, `components/settings/secrets/**`),
  separately documented.
- **Data Sources** (`ApiKeysSettings.tsx`/`OrgApiKeysSettings.tsx`) and
  **Notifications** (`NotificationsSettings.tsx`) are on the same sidebar but
  belong to the knowledge-store and notifications areas respectively, not
  covered here beyond noting they exist in the nav.
- **Quotas** (`ResourceLimitsSection.tsx`, rendered inside the Usage pages):
  a resource-quota system (`useAccountUsage`, `useQuotaIncreaseRequests`)
  distinct from billing spend metering, with no canonical doc yet. Flagged in
  this doc's Known gaps rather than documented here, since it wasn't in
  scope for this pass.

## Two parallel settings shells

`SettingsLayout.tsx` (personal, `/settings/*`) and `OrgSettingsLayout.tsx`
(`/settings/org/:orgSlug/*`) are separate components, not one shell
parameterized by scope. Each renders its own nav and its own set of
page components (`AuditLogSettings` vs `OrgAuditLogSettings`,
`ExperimentsSettings` vs `OrgExperimentsSettings`, and so on) that share
presentation components (`AuditLogView`, `SectionHeader`) but are otherwise
independent route trees. `SettingsRedirect` sends bare `/settings` to
`/settings/account`.

`OrgSettingsLayout` additionally:
- Resolves `:orgSlug` against `useAuth().accounts` and renders a 403 panel if
  the caller isn't a member (or a spinner during the resolve).
- Auto-switches the WorkOS session to the target org
  (`switchOrg(org.organization_id)`) if the JWT's current org doesn't match
  the URL. This is what makes `role`-gated nav items (`isOrgAdmin(role)`)
  correct for the org actually being viewed, not whichever org the session
  last had active.
- Gates Usage/Billing/Secrets/Data Sources/OAuth Apps/Audit Log/Experiments
  nav items on `isAdmin`; Account (the `general` route) and Members are
  visible to every member.

## One sidebar, two scopes (`SettingsSidebar`)

Both shells render the same `components/settings/SettingsSidebar.tsx`: a
"Settings" heading, an account scope selector, then whatever grouped nav the
shell passes as children. The selector is the only way to reach org settings
from the UI; the Organizations page lists orgs and offers "Leave", it does
not link into org settings.

Picking an account navigates to the same section in the target scope via
`settingsScopePath` (`lib/settings-paths.ts`). Sections that exist in both
scopes are preserved, the personal Account page and the org General route
stand in for each other, and anything with no counterpart (Connectors,
Organizations, Members) falls back to that scope's landing section. The
current section comes from the URL (`settingsSectionFromPath`), so the
selector needs no state of its own.

The selector navigates; it does not call `setActiveAccount`. Settings scope
is URL-derived, and the org shell already owns the WorkOS `switchOrg` for the
org in the URL, so the app-wide active account (agents list, deploy targets)
is deliberately left alone.

Nav items are grouped by `SidebarNavGroup` (Manage, Access, Integrations)
with `SidebarNavDivider` separating the trailing Experiments item. A group
renders `display: contents` below `md` so its children still flow into the
mobile pill row and dropdown as if ungrouped, and only the desktop column
shows the group label.

The two scopes are not identical menus. Connectors and Organizations are
personal-only, Members is org-only, and a section with no counterpart in the
other scope is simply absent there rather than shown disabled.

## Profile, avatar, and username (`ProfileEditor`, `AvatarUploadDialog`, `ChangeUsernameDialog`)

`ProfileEditor` edits an org's display name and avatar; `OrgGeneralSettings`
(`ProfileSection`) wraps it with an org-scoped save. `AccountSettings`
(personal) doesn't render `ProfileEditor` at all: it only offers
`ChangeUsernameDialog` for the account's username, with no display-name or
avatar editor on that page. `ProfileEditor` owns:

- Dirty-tracking with a navigation blocker (`useBlocker`) and a
  `beforeunload` handler, so an edited-but-unsaved display name isn't lost by
  an accidental route change or tab close.
- Client-side display-name validation (`getDisplayNameError`, aware of
  `displayNameKind`: `"personal"` vs `"organization"` have different length
  limits mirroring the backend's `DisplayNameMaxLength` and
  `OrganizationDisplayNameMaxLength`).
- Opening `AvatarUploadDialog` on avatar click; `readOnly` disables both the
  avatar button and the name field for a non-admin viewing an org.

`AvatarUploadDialog` is a two-step dialog (select, then crop) built on
`ImageUpload` + `ImageCropper` + `cropImage`. It does not talk to the API
itself; it hands the cropped `Blob` to the caller's `onUpload`, and calls
`onSuccess(blob)` after that resolves so the caller can locally bust the
avatar cache (`bustAvatar`) before the next server-truth refresh lands.

`ChangeUsernameDialog` (personal and org, via `variant`) wraps the generic
`ConfirmationDialog` with a debounced availability check
(`useAccountNameValidation` → `AccountNameInput`) and calls
`useRenameAccount`. Renaming is framed as destructive in the copy (breaks
existing links/CLI configs referencing the old name) because the backend
rename is exactly that. See
[`account-lifecycle.md`](account-lifecycle.md#account-creation).

## Danger zone: delete and leave

`DangerZoneItem` is the shared row (title, description, destructive button,
optional disabled-with-tooltip state) used by both `AccountSettings`
(personal: only "Delete account") and `OrgGeneralSettings` (org: "Leave
organization" and "Delete organization", the latter disabled for
non-admins).

- **`DeleteAccountDialog`** and **`DeleteOrganizationDialog`** are both thin
  wrappers around `ConfirmationDialog` + `useDeleteAccount`, requiring the
  user to type the exact account name/slug to confirm. Both call the same
  `DELETE /api/v1/accounts/:account` endpoint. See
  [`account-lifecycle.md`](account-lifecycle.md#soft-delete--purge-lifecycle)
  for what happens server-side (balance check, soft-delete, async teardown).
  On success, the personal-account dialog logs out and navigates to `/`; the
  org dialog just refreshes auth state and returns to the org list.
- **`LeaveOrganizationDialog`** is member-management, not account deletion.
  It calls `useRemoveAccountMember` (optionally preceded by
  `useUpdateMemberRole` to promote a replacement) against the *members* API,
  not the account-delete endpoint. It blocks leaving if the caller is the
  sole member (must delete instead) and, if the caller is the last admin,
  requires picking another member to promote to admin first. It is mounted
  twice: from `OrgGeneralSettings`' danger zone, and from each row of
  `OrganizationsSettings`.

## `OrgMembersSettings` and the invite flow

`OrgMembersSettings` lists members via `useAccountMembers(orgSlug, { includePending: true })`.
Pending WorkOS invitations are returned in the same list, distinguished by
`member.status === "pending"` (rendered as "Invited" instead of a role, and
"Revoke invitation" instead of "Remove member" in the row menu). Role changes
(`useUpdateMemberRole`) and removal (`useRemoveAccountMember`) are gated
client-side by `canManageMember`, which mirrors the backend's
`org.ErrOwnerManagementForbidden` rule: the caller must be admin/owner, can't
manage themself, and can't manage an owner unless they're an owner too. The
UI computes this defensively for immediate feedback; the backend is the real
enforcement point.

**`InviteMembersDialog`** takes a mix of emails and Astro usernames via the
shared `InviteInput` component (each entry tagged `kind` and validated
client-side before send), and posts them in one batch via
`useCreateInvitations` to the account's bulk-invitation endpoint. All invites
sent from this dialog are created with `role: "member"`: there's no role
picker at invite time; a member is promoted afterward from the members
table.

## `ConnectorsSettings`

One page, three independent sections (GitHub, Slack, Supabase), each backed
by its own query hooks (`api/queries/github.ts`, `slack.ts`, `supabase.ts`)
and its own OAuth round trip: `connect.mutate(...)` returns a
`redirect_url` the browser navigates to, the provider redirects back to
`/settings/connectors` with query params (`github_connected`, `slack_team`,
`supabase_error`, and so on), and `useCleanupOAuthParams` strips them from
the URL after the section reads them. Each section renders through the
shared `ConnectorRow`/`ConnectorRowList`/`ConnectorRowItem` presentation
components (icon, name, status/description, action, with an optional
expandable list of connected accounts/workspaces below).

This page is connection *management* only: linking/unlinking the OAuth
identity and, for GitHub, listing authorized orgs. The actual use of each
connection (GitHub repo-linked builds, Slack agent messaging, Supabase
knowledge-store import) is documented per-provider and cross-linked from
here rather than repeated:

- GitHub: [`github-connection.md`](github-connection.md)
- Slack: no dedicated architecture doc yet as of this pass. The connection
  surface here is `api/queries/slack.ts` and `internal/slackidentity`/Slack
  handlers server-side.
- Supabase: [`supabase-knowledge-store.md`](supabase-knowledge-store.md)

Notable behavior: disconnecting GitHub or Slack requires typing a
confirmation phrase (`ConfirmationDialog`) because it silently stops
automatic builds or agent messaging respectively; disconnecting Supabase does
not, because existing knowledge stores keep working and only new imports are
blocked.

## `AuditLogSettings` / `OrgAuditLogSettings`

Both are one-line wrappers around the shared `AuditLogView`, differing only
in which account slug they pass and the subtitle copy. `AuditLogView` itself:

- Paginates via `useAuditLog(account, { limit: 50, resource_type, action, actor_id })`
  (infinite query) and layers a client-side text filter
  (`debouncedSearch`, 250 ms) on top of the already-fetched pages. Search
  does not re-query the server, so "Load more" and search interact
  (`hasNextPage` pagination is hidden while a search term is active).
- Resolves each entry's actor to a name via `useAccountMembers(account)`
  (a `user_id → AccountMember` map) for `type: "user"` actors; `"system"`
  and `"admin"` actors are rendered from the raw actor ID.
- Filter option lists (`resource_types`, `actions`) come from a dedicated
  `useAuditLogFilters(account)` query, not derived from the currently loaded
  page, so the filter dropdowns list every value that occurs anywhere in the
  log, not just the visible page.

## `ExperimentsSettings` / `OrgExperimentsSettings`

This page is two genuinely different flag mechanisms sharing one UI, and
misreading it as "just a settings page with toggles" loses that distinction:

**1. Client-only, `localStorage`-backed flags** (`src/lib/experiments.ts`).
A tiny module-level store (`useSyncExternalStore`) persisted to
`localStorage["astro:experiments"]`, currently holding exactly one flag
(`evals`, gates the Eval tab on agent-detail pages). `hasExperiments` is
`Object.keys(DEFAULTS).length > 0`, literally "are there any client flags
left to show," which is what makes `ExperimentsSettings` disappear from the
sidebar entirely once the last one graduates or is removed (see
`SettingsLayout.tsx`'s `{hasExperiments && ...}` guard). Cross-tab changes
propagate via the `storage` event; same-tab changes propagate via a manual
`notify()`, because browsers don't fire `storage` in the writing tab.
**These flags are per-browser, not per-account**: they carry no server
state and no audit trail.

**2. Server-owned account experiments** (`internal/experiment`, table
`account_experiments`, one row per `(account_id, key)`; a missing row reads
as `false`, not an error). Exposed via
`GET/PUT /api/v1/accounts/:account/experiments/:experiment`
(`handlers/experiments.go`), gated per-slug by `experimentsBySlug`: currently
`fine-grained-access` (org-only; toggling it invalidates the deployment cache
because the deployment list reads the flag per request, see
[`fine-grained-access-control.md`](fine-grained-access-control.md)) and
`prompt-classification-stats` (available to personal accounts too; toggling
it invalidates cached Insights queries because it changes what the Insights
API returns). Every toggle writes an `account_experiment` audit log entry.
Unlike the `localStorage` flags, these are real account state, visible to
every member, and show up in the audit log.

`ExperimentsSettings` (personal) renders **both** kinds side by side: the
`localStorage` `evals` toggle, and the server-owned
`prompt-classification-stats` toggle scoped to the reader's personal account.
`OrgExperimentsSettings` renders only server-owned toggles
(`fine-grained-access` and `prompt-classification-stats`), scoped to the org.
There's no `localStorage` section on the org page, since those flags aren't
account-scoped at all. The personal page's own comment notes that once
`hasExperiments` is `false` (last client flag removed), the page must not
redirect away if a personal account still exists, because the server-owned
switch still needs a home.

## Known gaps

- No canonical doc for the quota/resource-limits system
  (`ResourceLimitsSection.tsx`, `useAccountUsage`, `useQuotaIncreaseRequests`)
  surfaced on the Usage settings pages. It's adjacent to but distinct from
  billing spend metering. Not documented here since it was out of this pass's
  scope; worth a dedicated pass.
- No architecture doc for the Slack connector/identity system
  (`api/queries/slack.ts`, `internal/slackidentity`) comparable to
  `github-connection.md` or `supabase-knowledge-store.md`.
- The settings design calls for a Groups section under Access in both scopes.
  The server has full access-group CRUD (`handlers/access_groups.go`) but the
  client has no page and no query hooks, so the nav item is not rendered yet.
- There is no org-scoped Connectors page. The route doesn't exist and
  `ConnectorsSettings` hardcodes `personalAccount` in all three sections
  (`ConnectorsSettings.tsx:77`, `:247`, `:455`). The API is not the blocker:
  the account routes already accept any account the caller belongs to, but
  every handler keys the credential off `session.UserID` rather than the
  account (`handlers/github.go:96`, `handlers/supabase.go:135`), so an org
  connection would still be one member's personal grant. Slack is a
  human-to-Slack identity link with no account column at all
  (`handlers/slack.go:380`) and doesn't belong in org scope. The same
  user-keying is what makes org builds fail when the linking member leaves,
  documented in [`github-connection.md`](github-connection.md#dependency-on-workos_user_id).
- Test coverage is thin around exactly the flows this doc leans on hardest:
  `AccountSettings.tsx`, `AuditLogSettings.tsx`/`OrgAuditLogSettings.tsx`,
  `ExperimentsSettings.tsx`, `ChangeUsernameDialog.tsx`, `DangerZoneItem.tsx`,
  `DeleteAccountDialog.tsx`, `DeleteOrganizationDialog.tsx`, and
  `InviteMembersDialog.tsx` all have no `.test.tsx` file. `OrgMembersSettings.tsx`,
  `ConnectorsSettings.tsx`, `AvatarUploadDialog.tsx`, `ProfileEditor.tsx`, and
  `OrgGeneralSettings.tsx` do.

## Verify

- `cd apps/astro-client && bun x vitest run src/pages/settings src/components/settings`: runs everything with a `.test.tsx` in scope (`ConnectorsSettings`, `OrgExperimentsSettings`, `OrgGeneralSettings`, `OrgMembersSettings`, `OrgSettingsLayout`, `SettingsLayout`, `SettingsSidebar`, `AuditLogView`, `AvatarUploadDialog`, `ConnectorRow`, `LeaveOrganizationDialog`, `ProfileEditor`), plus the billing/secrets tests that also live in these directories. It silently skips the untested files listed above rather than failing; a green run here is not full coverage of this doc.
- `cd apps/astro-client && bun x vitest run src/lib/settings-paths.test.ts`: the scope-selector path mapping (`settingsScopePath`, `settingsSectionFromPath`).
