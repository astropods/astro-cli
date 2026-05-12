## Summary

The Blueprints tab toolbar (search, visibility filter, sort, and Customize Order button) was visible even when a user had no blueprints yet, cluttering the empty state with controls that did nothing.

## Design

The toolbar is now conditionally rendered: it only appears when `blueprints.length > 0` or when the user has active filters (search, visibility, or sort). This means:

- **True empty state** (no blueprints, no filters active): toolbar is hidden entirely; only the empty state card with a "Create blueprint" CTA is shown.
- **Filtered empty state** (filters active, no matches): toolbar remains visible so the user can clear their filters.
- **Blueprints present**: toolbar renders as before, including the Customize Order button.

## Migration

No action required.
