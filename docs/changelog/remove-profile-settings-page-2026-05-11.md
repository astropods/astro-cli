---
title: Remove Profile tab from Settings
---

## Summary

The Profile tab in `/settings` duplicated the profile editor that already lives on the user's public Account Profile page (`/{username}`, via `ProfileEditSidebar`). Having two entry points for the same edits — display name, bio, social links, avatar — meant users had to learn which one to use and we had to keep two surfaces in sync. The Profile tab is removed; the Account Profile page is now the single place to edit a user's public profile.

## Design

The `/settings` area continues to host concerns that are *not* part of the public profile (Account, Usage, Variables & Secrets, Organizations, Audit Log, Experiments). The public profile editor on the Account Profile page is unchanged.

Concrete changes in `apps/astro-client`:

- `routes.ts` — drop the `profile` child route from the `/settings` layout.
- `pages/settings/SettingsLayout.tsx` — remove the Profile `SidebarNavItem`.
- `pages/settings/SettingsRedirect.tsx` — index redirect now points at `/settings/account` instead of `/settings/profile`.
- `pages/settings/ProfileSettings.tsx` — deleted.

The shared `ProfileEditor` component and the org-level profile section in `OrgGeneralSettings` are untouched.

## Migration

No action required. Any external link to `/settings/profile` 404s; users should be directed to their Account Profile page (`/{username}`) to edit profile fields. The `/settings` index continues to resolve — it now lands on Account.
