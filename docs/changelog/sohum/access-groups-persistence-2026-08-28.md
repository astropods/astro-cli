# Local group model

## Summary

Astro now has the persistence foundation for reusable account groups. It enables searchable group lists, creator-managed groups, and archived groups that can be restored.

## Design

`groups` gives every group a stable Astro ID and maps it to a WorkOS group ID. It records the description, creator, and lifecycle.

The creator manages the group. Account managers retain account-wide management access. WorkOS remains the source of truth for membership, so Astro does not duplicate the roster or maintain a synchronization ledger.

The creator is constrained to an account member. Names and avatars continue to resolve through Astro's existing account-member profile path.

The record supports group lifecycle APIs and future resource role assignments. Duplicate WorkOS IDs and duplicate active names remain distinct errors.

The persistence contract verifies creation, case-insensitive active names, and restore collisions against Postgres.

## Migration

The migration only adds new tables and indexes. Existing WorkOS groups and authorization behavior are unchanged until the lifecycle API adopts the model.
