# Summary

Populate WorkOS org membership id (`om_*`) on the server session so upcoming FGA checks have a subject. Replace the coarse deployment action with `deployment:read` and `deployment:edit`; there is no authorization behavior change because routes still use existing JWT permissions and membership checks.

# Design

WorkOS FGA evaluates checks against **organization membership ids**, not user ids. Add `Session.WorkOSMembershipID`, parsed from the JWT claim `organization_membership_id` when present.

Resolution is two-tier:
- **JWT claim first** — WorkOS Dashboard (Authentication → Features → JWT Template). Preview already has:

```json
{
  "organization_membership_id": "{{ organization_membership.id }}"
}
```

- **DB fallback** — on cookie session build (login callback, switch-org, refresh), look up `account_member_workos` via `GetByWorkOSOrganizationID` + `GetMember` when the claim is missing. Switch-org and refresh run membership sync first so the fallback sees up-to-date rows. `/auth/me` only reads the existing access-token claim and reseals valid legacy cookies when that claim is present, avoiding DB reads on this hot path. Resolution failures are logged rather than silently treated as an absent membership.

Bearer auth reads the JWT claim only (no per-request DB hit). `SubjectFromContext` maps `session.WorkOSMembershipID` → `authz.Subject.MembershipID` for PR 3+ FGA wiring.

The deployment FGAC rollout is documented alongside this identity prerequisite. The sequence is API-first: live resource writes, shadow checks, backfill, enforcement, access/group APIs, and preview verification precede any frontend work.

`deployment:read` covers reading non-secret deployment control-plane data. `deployment:edit` covers changing or operating a deployment. These values are WorkOS permission-slug contracts, but `MembershipChecker` still ignores the requested action until live FGA checks are introduced.

Org-unscoped sessions (first login before switch-org) legitimately have empty membership until the user selects an org.

Deliberately out of scope for this slice: WorkOS FGA SDK and checks, Dashboard resource types, route middleware and enforce flags, deploy-time owner persistence, client `AuthResponse` / `useCan`, and prod JWT template (preview only for now). This PR only resolves `om_*` onto the session and wires `Subject.MembershipID` for later FGA work.

# Migration

Preview WorkOS JWT template is updated (see Design). Repeat the same claim in prod before FGA enforce. Existing organization-scoped sessions with the JWT claim are hydrated and resealed on `/auth/me`; sessions without it pick up the membership on the next switch-org, token refresh, or re-login.
