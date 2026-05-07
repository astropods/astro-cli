## Summary

Organization vault REST APIs were gated with **`org:admin`**, which only **owners** carry—WorkOS **admins** could not list or edit secrets while still managing members. Vault is now authorized with dedicated WorkOS permission slugs **`variable:read`** and **`variable:write`**, granted to **owner** and **admin** roles only.

## Design

- **Permissions**: `variable:read` for `GET /accounts/:account/variables` and `GET .../variables/:varName`; `variable:write` for `POST`, `PUT`, and `DELETE`. Reads and writes are separate route groups, each with `RequireAccountPermission(...)` for the matching slug (same pattern as other fine-grained checks).
- **Personal accounts**: Unchanged—membership still implies full access; JWT permission lists are not consulted for personal targets.
- **WorkOS**: Operators must define both slugs in the dashboard and attach them to **owner** and **admin** role definitions so refreshed JWTs include the claims (see `docs/05-implementation/org-rbac-setup.md`).

## Migration

- Add **`variable:read`** and **`variable:write`** to **owner** and **admin** roles in WorkOS for each environment before or when deploying this server version.
- Org **members** without these claims continue to receive **403** on vault APIs (deploy UIs that surface load errors already explain failures).
