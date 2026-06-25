## Summary

The Eval tab needed to work across small, medium, and wide formats without forcing desktop-only sidebars or table columns. This change makes the review queue and dataset views adapt to the available content width while keeping the existing tab structure and trace panel behavior.

## Design

Eval now follows the other agent tabs' responsive shell pattern: a fixed header offset, a masked scroll surface, and container-query-driven content. The review queue stacks its queue list above the trace detail on narrow containers, then returns to a side-by-side workflow when space allows. The dataset view stacks the grade summary above the table on smaller widths, and dataset rows become readable block rows before returning to the full desktop table.

## Migration

No migration required.
