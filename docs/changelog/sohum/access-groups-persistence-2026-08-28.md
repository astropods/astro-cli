# Local group model

## Summary

Astro now has the persistence foundation for reusable account groups. It enables the Groups settings experience: searchable group lists, member previews, group administrators, archived groups that can be restored, and clear creation and membership attribution.

## Design

`groups` gives every group a stable Astro ID and maps it to a replaceable WorkOS group ID. It records the description, creator, lifecycle, and synchronization health.

`group_memberships` models the many-to-many relationship between groups and account members. Memberships carry a local `member` or `admin` governance role plus add/remove attribution. A group creator is inserted as its first administrator in the same transaction.

Only user IDs are stored. Names and avatars continue to resolve through Astro's existing account-member profile path, so profile changes require no group data rewrite. Account-scoped foreign keys prevent cross-account membership.

These records support group lifecycle APIs, member administration, archived-group restoration, and future resource role assignments. Text limits count characters rather than UTF-8 bytes. Duplicate WorkOS projections and duplicate group names remain distinct errors. Restore collisions and repeated member additions preserve archived names and administrator roles.

The persistence contract is verified against Postgres itself, including transactional creation, case-insensitive active names, restore collisions, and membership resurrection. This protects the database invariants that query mocks cannot exercise.

## Migration

The migration only adds new tables and indexes. Existing WorkOS groups and authorization behavior are unchanged until the lifecycle API adopts the model.
