# Summary

Signed-out visitors could reach the public Astro Pods Explorer, but some Blueprints links sent them to the account-scoped blueprints surface. That route depends on authenticated account context and could fail instead of keeping visitors in public discovery.

# Design

Blueprint navigation now separates public discovery from account management. Signed-out visitors see no Blueprints nav tab at all — public discovery is reached through the existing Explore action — while authenticated users continue to use the account-scoped blueprints dashboard. This avoids surfacing a Blueprints tab that, for the unauthenticated, only duplicated Explore.

Focused regression tests cover the signed-out header (no Blueprints tab, Explore still reachable) and the direct blueprint-detail breadcrumb so the public and authenticated nav contracts stay distinct. A marketing/public smoke test asserts the signed-out top navbar omits the Blueprints tab.

# Migration

No user action required.
