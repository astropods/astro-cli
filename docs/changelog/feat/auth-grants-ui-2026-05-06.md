# Auth grants UI

## Summary

The per-deployment authorization grants introduced in [authorization-2026-04-25](../authorization-2026-04-25.md) had no UI surface — owners could only express access policy by hand-editing the deployment spec. This change adds an editor inline in the deploy/configure form so owners can manage `interfaces.auth.{web,slack}.grants` directly. While we were touching the spec, the awkward `account_id` field name was renamed to `org` to match the user-facing language.

Web auth is now always-on OIDC (the "Require authentication" toggle is gone). Removing the toggle simplifies the model: web clients always go through OIDC, and grants decide who is authorized post-login.

## Design

**Inline per-adapter editor.** Each selected adapter's card in `InterfacesPicker` hosts a `GrantsEditor` showing the current grants list and an "Add access" affordance. Slack puts the editor *above* the bot/app-token credentials — the access decision (who can use the bot) is the more important first question. Web shows credentials first since web typically has none.

**Three subject types, scoped by adapter:**

- **Anyone** — public; available on both web and slack.
- **Members of organization** — submenu of the user's org accounts (personal accounts are filtered out: their member set collapses to one user, which the user picker covers).
- **Specific user** — web only; backend rejects `user_id` on slack since slack identity is opaque.

**User picker searches by account name.** Selecting "Specific user…" opens an inline `MemberPicker` over `useAccountMembers(targetAccount)` that filters by `username` or `display_name`. Picking a member adds a `user_id` grant; the row resolves back to `@username` for display.

**Avatars throughout.** Org and user grants render with `UserAvatar`; "Anyone" gets a globe-in-circle badge. Each grant row shows display name as primary text and `@handle` (or "Members of @handle") as secondary, so identity is unambiguous when display names overlap.

**Grants ride the existing template request roundtrip.** No new endpoint — `useDeployForm` adds `webGrants` / `slackGrants` to its state, seeds them from the prefilled `TemplateResponse.interfaces.auth`, and emits them in `TemplateRequest.interfaces.auth.{web,slack}.grants` on submit. Web auth always emits `auth.web.type = "oidc"` when the web adapter is selected. Grants for deselected adapters are not sent, so deselecting Slack drops Slack grants cleanly.

**Spec rename: `account_id` → `org`.** The grant subject field, the DB subject-type constant (`SubjectTypeAccount` → `SubjectTypeOrg`, value `'account'` → `'org'`), and the CHECK constraint were all renamed. Spec form:

```yaml
interfaces:
  auth:
    web:
      type: oidc
      grants:
        - org: <account-uuid>            # was: account_id
        - user_id: <workos-user-id>
        - anyone: true
    slack:
      grants:
        - org: <account-uuid>
        - anyone: true
```

## Migration

No action required. The `org` field still holds the same account UUID value the old `account_id` field did; only the key name changed in the spec, the store, and the schema CHECK constraint. The rename is safe because no production rows used the previous value yet.

The "Require authentication" toggle is gone — existing deployments with web selected continue to use OIDC; new deployments get OIDC by default with no choice.
