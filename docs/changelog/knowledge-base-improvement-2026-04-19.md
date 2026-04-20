# fix: show K8s events in knowledge store detail page

## Summary

The Event log section on the knowledge store detail page only ever displayed a static "Store created" entry. Real K8s provisioning events (pulls, scheduling, warnings) were already fetched by the backend during provisioning but the frontend component never rendered them.

## Design

**Backend** — The `GetKnowledgeStore` handler gated K8s event fetching on `StatusProvisioning` only. Stores in `connecting` or `pending-acceptance` status also emit useful events, so the condition now includes all three transitional statuses.

**Frontend** — `EventTimeline` was entirely hardcoded. It now reads `store.events` from the API response and renders each event with type-appropriate icons (warning / info), reason, message, and repeat count — matching the existing pattern in `NewKnowledgeStore.tsx`. The static milestone entries (Store created, Connection verified, PrivateLink established) are preserved below the live events.

## Migration

No migration required.
