# Organization access-group domain

## Summary

Astro now has a transport-independent WorkOS Groups contract for organization group lifecycle and membership. Deployment access can validate an existing group as a same-organization assignment subject without adding HTTP routes or changing current behavior.

## Design

WorkOS remains authoritative for groups, memberships, and group role assignments. The shared WorkOS client exposes cursor-safe group and member pagination, lifecycle mutations, typed errors, and strict test fakes. Astro-owned cursors retain the exact position when an API page ends inside a WorkOS page, preventing skipped or duplicated entries.

The deployment access service resolves group subjects through their WorkOS organization before recording durable desired-role state. Cross-organization and missing groups remain concealed, while the existing reconciler applies group assignments asynchronously and retries failures.

PR8.4B will expose organization-admin group APIs behind the global FGA switch and organization experiment.

## Migration

No database migration or configuration change is required. This PR adds no routes.
