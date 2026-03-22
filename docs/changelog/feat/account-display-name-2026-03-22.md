# Add display_name to accounts

## Summary

Accounts now store a `display_name` directly in our database instead of relying on WorkOS `first_name`/`last_name` fields. This gives users and organizations a single, unsplit display name field that we fully control.

## Design

A `display_name varchar(64)` column is added to the `accounts` table. The field is set during account creation (onboarding for personal accounts, org creation for organizations) and editable via `PATCH /me` for personal accounts.

The server's `UpdateProfile` handler no longer calls WorkOS to update user fields — it updates the account's `display_name` in our DB directly. All API responses (`AuthAccountResponse`, `AccountResponse`, `AccountWithRoleResponse`, `ProfileResponse`) now include `display_name`.

On the client, all components that displayed user names (`AppHeader`, `UserCard`, `BlueprintsSidebar`, `AccountProfile`, `AccountSettings`) now read from `account.display_name` instead of the WorkOS user's `first_name`/`last_name`. The legacy `auth-utils.ts` utilities (`getUserDisplayName`, `splitDisplayName`, `getUserInitials`) were removed.

Onboarding now also requires agreeing to TOS and privacy policy before account creation.

## Migration

Atlas will add the `display_name` column with a default of `''`. Existing accounts will show their username as fallback until a display name is set. No manual data migration is required.
