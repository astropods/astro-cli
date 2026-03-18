# Refresh Agent Detail Sidebar Layout

## Summary

Refines the agent detail sidebar to match the updated visual direction with carded sections and clearer hierarchy. The goal is to improve scanability and align the install/hire panel with the target browsing experience.

## Design

The sidebar now uses a reusable section shell with a distinct header strip and neutral body surface so every block reads as its own card. Content that previously appeared as separate blocks was regrouped into a single "What it needs" section that nests connected apps and permissions. The details area was restructured into row-style key/value items, and support was added for optional metadata (rating, installs, teammate install counts) so the UI can progressively adopt richer catalog signals when data is available.

Microcopy and identity treatments were adjusted to better match the new tone, including the primary CTA wording and account handle display format.

## Migration

No migration steps required. Existing consumers continue to work with the current sidebar props, and newly added metadata fields are optional.
