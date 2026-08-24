# Queen deployment access inspector

## Summary

Queen can now explain who has access to an organization deployment, which permissions are effective, and whether that access comes from an organization role, a direct deployment assignment, or a group. The view is read-only and intended for operators diagnosing fine-grained access behavior.

## Design

The deployment detail page adds an Access tab backed by a dedicated admin request. It loads only when opened, so Queen's existing five-second deployment refresh never turns into repeated WorkOS authorization traffic. This is internal operational visibility, not a customer-facing assignment surface, and remains read-only regardless of the organization experiment state.

Astro remains the boundary to WorkOS. The admin backend pages through the complete organization roster, reads resource-role assignments once, and performs one effective-membership lookup for each of the five deployment permissions. It joins that live evidence with local member emails, includes members with no deployment access, and orders organization owners/admins before deployment admins, builders, viewers, and unassigned members.

The table shows organization roles, deployment roles, exact permissions, and grant sources. Direct, group-derived, and organization-inherited paths can appear together because WorkOS may combine them into one effective decision. Queen does not create, change, or revoke access.

## Migration

No schema or user migration is required. WorkOS configuration is reused from the existing FGA rollout; personal deployments, unregistered historical deployments, and servers without FGA configuration show an explanatory empty state.

To verify in Preview, open Queen → Deployments, select an organization deployment, and open Access. Confirm the roster contains every organization membership, an organization owner/admin shows inherited access, a direct Viewer shows only `deployment:read`, a group Builder shows the Builder permission bundle, and an unassigned member shows no deployment access. Change an assignment through the Astro API, select **Refresh evidence**, and confirm the explanation updates.
