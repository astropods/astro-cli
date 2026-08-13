# Effective deployment capabilities

## Summary

Astro now has one authenticated API for answering which deployment actions the current caller may perform. This establishes a stable client contract before any existing route starts denying requests through FGA.

## Design

`GET /api/v1/deployments/:id/capabilities` returns the five deployment permissions as booleans. For an eligible PR4-synchronized organization deployment, Astro lists the membership's live effective WorkOS permissions once, including direct, group-derived, and inherited organization access. When the FGA path is inactive, the response identifies legacy mode and preserves today's member capabilities; `deployment:manage_access` remains false because no legacy access-management API exists.

Astro first verifies current account membership and deployment ownership. On the live FGA path, it also confirms the session organization matches the resource organization before calling WorkOS; cross-organization FGA requests are concealed as not found. Legacy mode intentionally mirrors existing deployment endpoints, whose authorization boundary is account membership. Missing identity or WorkOS failures return a retryable unavailable response. Permission decisions are never cached between requests.

This PR reports capabilities only. Existing read and mutation endpoints keep their current authorization behavior; the stacked enforcement PR uses this same tenant-safe checker and resource gate.

Deployment access is private by default, following GitHub's private-repository model with no organization-wide base permission. Organization owners and admins inherit access to every deployment, the creator receives a direct `deployment-editor` assignment, and ordinary members receive no deployment access until a person or group is assigned a resource role. This preserves per-deployment privacy because WorkOS grants are additive and has no deny rule.

The rollout remains split into three reviewable changes: PR7.1 establishes capability reporting, PR7.2 adds the server-owned organization opt-in and organization Settings → Experiments control, and PR7.3 makes the reviewed mutation routes enforce WorkOS only for opted-in organizations. Turning the organization experiment off retains legacy membership behavior and shadow observation. Historical Preview deployments are backfilled only after PR9; the opt-in control never starts a partially applied migration itself.

## Review plan

- Confirm the endpoint makes one live effective-permissions request for an eligible organization deployment.
- Confirm direct, group-derived, and inherited permissions map to the five booleans.
- Confirm denied and missing resources are concealed, and cross-organization FGA requests never call WorkOS with the wrong tenant.
- Confirm personal, historical, and unconverged deployments retain explicit legacy behavior.
- Confirm no existing endpoint gains an FGA denial in this PR.

## Preview test

Call the capability endpoint as an editor, a reader, and an unassigned organization member. Editors should receive their complete bundle, readers only `deployment:read`, and unassigned members should receive the same not-found response as a nonexistent deployment. Change a WorkOS assignment and repeat the call to prove the response changes live.

## Migration

No user action is required. Preview continues using the existing FGA shadow configuration; production behavior remains unchanged.
