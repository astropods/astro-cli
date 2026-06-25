## Summary

The eval dataset and trace detail views get a small readability pass. The goal is to make expanded rows and trace content feel clearer without changing the review workflow or adding new controls.

## Design

Trace content sections now use a stronger border token so the user and agent response containers read as distinct cards against the dark background. The internal divider uses the same stronger border so the header and body stay visually connected.

Expanded dataset rows keep the primary left accent that shows which row is open, but no longer fill the whole row with the primary-tinted background. This keeps the focus indicator without making the expanded state feel heavy.

The dataset table now uses fixed layout with explicit column widths for the summary row. Input and expected output get more balanced space, while verdict and reviewer columns keep predictable widths for scanning.

The eval page now uses the same fade-and-lift entrance used by the other agent detail tabs, and the review queue/dataset sub-tabs transition through the same motion pattern instead of swapping abruptly. This keeps the eval workflow feeling consistent with Monitor, Deployments, and Configure while preserving the existing review and dataset layouts.

## Migration

No user action is required. These are presentation-only changes to existing dataset and trace detail surfaces.
