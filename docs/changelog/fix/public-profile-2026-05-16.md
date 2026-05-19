## Summary

Owners viewing their own account profile via the "View as visitor" link (`/<account>?visitor`) still saw internal-only sidebar content — the Agents stat on personal profiles, and the Members section on org profiles — even though the Agents tab and private-blueprint counts correctly hid in that mode. A truly-unauthenticated viewer never saw either (the deployments API isn't called for non-members; the members API requires membership and returns nothing), but the owner's preview did, defeating the purpose of the visitor toggle as a quick public-view sanity check.

## Design

The Agents stat in `ProfileViewSidebar` was gated by a `canViewDeployments` prop sourced in `AccountProfile` from component scope (`isSelf || isOrgMember`), which has no awareness of the `?visitor` query param. Meanwhile `ProfileLayout` already computes a visitor-mode-aware `isInternalView = canViewDeployments && !isVisitorMode` and uses it to gate the Agents tab and the public-blueprint count.

`SidebarRenderOpts` now exposes `isInternalView`, populated by `ProfileLayout`'s derived value, and `AccountProfile` reads it from the render opts instead of its own component scope. `ProfileViewSidebar`'s `canViewDeployments` prop is renamed to `isInternalView` since its value already represented that broader concept, and the Members section is gated by the same flag. An owner's `?visitor` preview now matches what a true non-member viewer would see across both stats and member list.

## Migration

No action required. Visible behavior only changes for the owner/admin in `?visitor` mode, where the Agents stat (personal) and Members section (org) now correctly hide.
