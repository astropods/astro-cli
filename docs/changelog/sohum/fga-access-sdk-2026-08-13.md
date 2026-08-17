# Fine-grained access SDK primitives

## Summary

This change adds the WorkOS-backed assignment and resource-discovery contracts needed by the next deployment-access APIs. It also replaces the deployment-scoped Owner name with Admin; organization/account ownership remains unchanged. It introduces no routes or permission-policy change.

## Design

Astro keeps product concepts separate from vendor transport:

- Viewer, Builder, and Admin remain configured WorkOS deployment roles; Astro checks their permissions rather than duplicating their bundles locally.
- Resource assignments stay behind `AccessAssignments`; malformed WorkOS assignment records fail visibly instead of producing an incomplete access list.
- Readable-resource discovery has its own `ResourceDiscovery` contract so list handlers do not depend on the full lifecycle client.
- The process-wide WorkOS client implements both contracts with the official SDK, and strict fakes keep downstream tests isolated.

The string values for resource types, roles, and permissions remain external contracts with the WorkOS environment configuration.

Group lifecycle and its pagination intentionally move to PR8.4, where the first group API consumes them. This keeps the foundation limited to code used by PR8.2 and PR8.3.

## Review plan

1. Confirm this layer contains no HTTP route or enforcement changes.
2. Confirm every WorkOS operation validates required identifiers and uses the official SDK.
3. Confirm malformed assignment records fail rather than silently disappearing.
4. Confirm strict fakes reject unexpected assignment and discovery calls.

## Migration

Before this code reaches any environment:

- Create `deployment-admin` on the Deployment resource type with all five deployment permissions.
- Keep `deployment-owner` through the PR8 rollout.
- Verify a newly created deployment assigns Admin to its creator before deleting Owner.

No assignment backfill is required in this PR. A startup role lookup is intentionally avoided: it would add a WorkOS network dependency to process health, while reconciliation already records and retries a missing-role failure.
