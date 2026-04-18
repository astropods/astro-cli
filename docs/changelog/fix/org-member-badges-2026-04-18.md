# Summary

The Organizations settings page (`/settings/organizations`) no longer shows role badges next to each org. This restores the owner/admin/member tags that indicate the user's membership role in each organization they belong to.

# Design

The root cause was that the server's auth response (`/auth/me`, `/auth/refresh`, `/auth/switch-org`) included accounts without per-org roles. The session-level `role` field only reflects the currently-active org, so a client-side fallback could show at most one badge at a time.

The fix adds per-org roles on the server side:

- `org.Sync.GetMembershipRoles` calls WorkOS `ListOrganizationMemberships` for the user and returns a `WorkOSOrganizationID → role` map
- `AuthAccountResponse` gains a `role` field (`json:"role,omitempty"`)
- `fetchAccounts` in the auth handler enriches each org account with its role from the map; personal accounts get no role field

On the client, `OrganizationsSettings` reads `org.role` directly and maps it through `ORG_ROLE_TAG`:

- `owner` → yellow tag
- `admin` → blue tag
- `member` → default tag

The tag is rendered below the org name rather than inline with it. Unknown or missing roles render no tag.

E2e coverage added to `org-permissions.spec.ts` for all three role badge states using the existing `/test/set-role` mock endpoint.

# Migration

No migration required. The `role` field is additive and `omitempty`; existing clients that ignore it are unaffected.
