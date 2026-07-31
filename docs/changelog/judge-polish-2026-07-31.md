# Eval review interface refinements

## Summary

The Eval review experience now communicates judge availability more clearly and presents verdict actions with consistent visual hierarchy. Related layout refinements improve alignment, wrapping, and information density across the review queue and Dataset tab.

## Design

- Disabled the AI judge action while judging or when no eligible traces remain.
- Added an explanatory tooltip when every trace already has a verdict.
- Made **Agree with judge** the primary action and placed it to the right of **Disagree**.
- Reduced the width of the alternative-verdict menu.
- Aligned the Bad verdict color between queue and detail views.
- Corrected segmented-control spacing and inner corner radii.
- Removed the internal dataset identifier from the Dataset tab header.
- Improved the Eval page description wrapping.

## Migration

No migration is required.
