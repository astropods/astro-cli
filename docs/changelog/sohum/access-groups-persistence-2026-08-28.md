# Local access-group model

## Summary

Astro now has a canonical data model for reusable account groups. WorkOS can remain the authorization projection while Astro owns product metadata, group administrators, lifecycle state, attribution, and future classification context.

## Design

`access_groups` gives every group a stable Astro ID and maps active groups to a replaceable WorkOS group ID. It records the group description, creator, lifecycle, management source, classification metadata, and synchronization health.

`access_group_memberships` models the many-to-many relationship between groups and account members. Memberships carry a local `member` or `admin` governance role plus add/remove attribution. A group creator is inserted as its first administrator in the same transaction.

Only user IDs are stored. Names and avatars continue to resolve through Astro's existing account-member profile path, so profile changes require no group data rewrite. Account-scoped foreign keys prevent cross-account membership.

## Migration

The migration only adds new tables and indexes. Existing WorkOS groups and authorization behavior are unchanged until the lifecycle API adopts the model.
