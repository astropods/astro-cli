# Local access-group model

## Summary

Astro now has the persistence foundation for reusable account groups. It enables the Groups settings experience: searchable group lists, member previews, group administrators, archived groups that can be restored, and clear creation and membership attribution.

## Design

`access_groups` gives every group a stable Astro ID and maps active groups to a replaceable WorkOS group ID. It records the group description, creator, lifecycle, management source, classification metadata, and synchronization health.

`access_group_memberships` models the many-to-many relationship between groups and account members. Memberships carry a local `member` or `admin` governance role plus add/remove attribution. A group creator is inserted as its first administrator in the same transaction.

Only user IDs are stored. Names and avatars continue to resolve through Astro's existing account-member profile path, so profile changes require no group data rewrite. Account-scoped foreign keys prevent cross-account membership.

These records also support the next product layers without changing the model: group lifecycle APIs can project membership to WorkOS, access flyouts can assign a whole group to resources, and Insights can use classification metadata for group-aware reporting. Classification metadata is intentionally open-ended; Astro requires a valid JSON object but does not impose a classifier schema yet. Restore collisions and repeated member additions are handled safely so archived names and administrator roles cannot be overwritten accidentally.

The persistence contract is verified against Postgres itself, including transactional creation, case-insensitive active names, restore collisions, and membership resurrection. This protects the database invariants that query mocks cannot exercise.

## Migration

The migration only adds new tables and indexes. Existing WorkOS groups and authorization behavior are unchanged until the lifecycle API adopts the model.
