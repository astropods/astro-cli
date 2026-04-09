# Fix GitHub build token retrieval for org-scoped users

## Summary

GitHub builds were failing with `get github token: not_installed` for users authenticated within a WorkOS organization. The build job was calling WorkOS Pipes `GetAccessToken` without an `OrganizationID`, but the token was originally issued in an org-scoped session context — causing WorkOS to return `not_installed` when looking it up server-side without that context.

## Design

The `github_connections` table now stores `workos_org_id` alongside `workos_user_id`. At link time (`GitHubLink`), `session.OrganizationID` is persisted to the connection record. The background build job (`GitHubBuildWorker`) then passes both `UserID` and `OrganizationID` when calling `GetAccessToken`, matching the context under which the pipe was originally authorized.

Personal accounts (no org) store an empty string, which is equivalent to the previous behavior.

Also added structured error logging on `GetAccessToken` failures in the build job and rebuild handler to make future token issues easier to diagnose.

## Migration

Atlas will apply `ALTER TABLE github_connections ADD COLUMN workos_org_id text NOT NULL DEFAULT ''`. Existing rows get an empty string, which is correct for personal accounts. Users with org-scoped connections will need to disconnect and re-link once to populate the column.
