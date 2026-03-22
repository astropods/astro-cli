## Summary

This change improves usability of the deployed-agent detail experience on smaller viewports by making monitor and deployment surfaces adapt without clipping, reducing nested scroll behavior, and keeping controls visually consistent.

## Design

The detail view now favors a single-page scroll model and responsive composition over fixed panel heights and rigid table layouts.

- Monitor and Deployments sections were adjusted to avoid horizontal cutoff, including compact layouts that hide lower-priority columns and improve label density.
- Traces now support progressive disclosure (`See more` / `See less`) to reduce initial vertical footprint while still allowing full inspection.
- The Configure experience now has two presentation modes:
  - large viewports: right-side popout that condenses the main content
  - smaller viewports: full-page configure mode without popout squeeze
- Badge and dropdown styling were normalized for stronger visual consistency across controls.
- Developer-only mock traces were expanded to provide realistic volume for UI tuning and overflow validation.

## Migration

No migration steps are required. This is a UI-only change with no API, schema, or config contract changes.
