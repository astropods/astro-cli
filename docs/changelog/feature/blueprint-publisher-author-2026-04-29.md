# Blueprint Authors from Audit Log

## Summary

The Authors section in the blueprint detail sidebar previously showed whoever was listed in the AGENT.md frontmatter, or fell back to the org account name. This surfaces everyone who has ever pushed the blueprint spec as the authoritative author list, pulling directly from the audit log.

## Design

A new `DistinctActorsFor` method on `auditlog.Store` returns all unique actor IDs that performed `agent.register` on a given agent, ordered by their first push ascending (original author first). The `GetAgent` handler calls this, resolves each actor via WorkOS, and returns a `publishers []AgentPublisher` array in the response. Name resolution falls back to the account handle if WorkOS has no first/last name for the user.

A new index `idx_audit_logs_creator_lookup` on `(account_id, action, resource_type, resource_id, created_at ASC)` covers the query efficiently.

On the client, `Blueprint.publishers` is typed as `BlueprintAuthor[]` (reusing the existing type — `BlueprintPublisher` was a duplicate and is removed). `SidebarCard` prioritizes audit log publishers over AGENT.md authors when both are present. The existing `SidebarAuthor` component already handles the single/compact/full-card display logic for multiple authors (threshold: >3 switches to avatar-only with tooltips).

## Migration

Atlas will apply the new index. No application changes required. Blueprints without audit history continue to fall back to AGENT.md authors, then the account owner.
