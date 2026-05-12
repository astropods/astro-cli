# Scope switcher shows display name

## Summary
The account scope switcher previously rendered the `name` (handle) for both personal and organization accounts. We now render `display_name` when present, with `name` as the fallback — matching how the avatar already labels itself and giving users a friendlier identifier in the chrome.

## Design
Two text spans in `OrgSwitcher` (the trigger label and the dropdown item label) now read `account.display_name || account.name`. The fallback keeps legacy accounts and personal accounts created outside the onboarding form (where `display_name` is not BE-required) rendering correctly.

The `Account.display_name` field is already optional in the API types and treated as such by `AccountIcon`, so no type or contract changes are needed.

## Migration
None required.
