# Fine-grained access hardening and architecture

## Summary

Phase 9 turns the deployment FGA implementation into a stable platform contract. It adds one stateful authorization journey across groups, direct assignments, reconciliation, discovery, enforcement, and revocation; removes superseded access-service surface; and replaces transitional rollout notes with permanent architecture and operations guidance.

This PR does not add frontend components or backfill historical deployments. The global kill switch, organization experiment, membership fallback, and shadow comparison remain because they are active rollback controls rather than dead code.

## Design

The contract suite drives the real Astro domain layers against one stateful WorkOS authority. A creator begins as deployment Admin, an unassigned member is denied, a group Builder grant becomes discoverable and enforceable after reconciliation, group removal revokes it, a direct Viewer grant permits only reads, and direct revocation removes visibility again. This protects the relationships between individually tested components instead of duplicating their unit cases.

Cleanup follows runtime evidence. The unused assignment-only `AccessService.List` entry point is removed in favor of `ListAccess`, which resolves tenant scope once and returns effective assignments with durable intent status. Runtime messages and comments now describe full deployment authorization rather than the earlier mutation-only rollout. Shadow and legacy fallback code remains intentionally reachable through configuration.

The permanent architecture defines:

- flat deployment permissions and Viewer, Builder, and Admin role bundles;
- direct, group-derived, and organization-inherited access;
- WorkOS versus Astro sources of truth;
- JWT organization scope versus live resource decisions;
- lifecycle and access-intent reconciliation;
- list discovery, caching, capabilities, tenant isolation, and failure behavior;
- global and organization rollout gates plus data-preserving rollback;
- the extension path for blueprint-parented deployments, knowledge stores, and future resources.

## Review

- Confirm the contract test proves both group-derived and direct access through grant, enforcement, discovery, and revocation.
- Confirm no effective entitlement or permission decision is sourced from an Astro database ledger.
- Confirm denied resources stay concealed, active failures fail closed, and cross-tenant validation precedes WorkOS calls.
- Confirm the global switch, organization experiment, shadow comparison, and legacy membership checker remain available for rollback.
- Confirm the architecture distinguishes current organization-parented deployments from the future blueprint-parented hierarchy.

## Migration

No schema, WorkOS Dashboard, environment, or user migration is required. Historical deployment backfill remains a separate, explicitly deferred milestone.
