# WorkOS Dashboard — Org RBAC Setup

One-time setup for a new WorkOS environment (preview, then prod). For how
organization roles and permissions work once configured, see
[`../03-architecture/organizations.md`](../03-architecture/organizations.md);
for the deployment-level authorization these permissions sit alongside, see
[`../03-architecture/fine-grained-access-control.md`](../03-architecture/fine-grained-access-control.md).

## 1. Configure roles and permissions

Under **WorkOS Dashboard → Organizations → Roles**:

| Role | Slug | Permissions |
|---|---|---|
| Owner | `owner` | `agents:read`, `agents:write`, `deployments:read`, `deployments:write`, `org:manage`, `variable:read`, `variable:write` |
| Admin | `admin` | `agents:read`, `agents:write`, `deployments:read`, `deployments:write`, `org:manage`, `variable:read`, `variable:write` |
| Member | `member` | `agents:read`, `agents:write`, `deployments:read`, `deployments:write` |

Steps:

1. Define permission slugs `variable:read` and `variable:write` (exact
   navigation depends on your AuthKit/Roles UI version).
2. Configure each role with the permission slugs above. Attach both vault
   permissions only to **owner** and **admin** — members get neither.
3. Set `owner` as the default role for organization creators.
4. Verify `WORKOS_API_KEY` and `WORKOS_CLIENT_ID` are set for the
   environment.

## 2. Add the FGA membership-id claim to the JWT template

WorkOS FGA checks use the caller's **organization membership id** (`om_*`),
not the user id. Adding it to the access token template lets astro-server
populate `Session.WorkOSMembershipID` without a DB lookup on every request.

1. In **WorkOS Dashboard**, select the target environment (preview first,
   then prod before turning FGA enforcement on).
2. Go to **Authentication → Features → JWT Template**.
3. Merge this claim into the existing template JSON:

   ```json
   "organization_membership_id": "{{ organization_membership.id }}"
   ```

4. Validate/preview, then save. The claim only resolves when the session is
   **org-scoped** (after `POST /auth/switch-org`) — an unscoped login
   session legitimately omits it until the user picks an org.
5. Existing sessions pick up the claim on their next token refresh, org
   switch, or re-login. astro-server falls back to `account_member_workos`
   when the claim is absent (cookie session build paths only).
