## Summary

The traces table and trace detail panel had several small visual inconsistencies. Resolved user display names rendered larger than the rest of the row, collapsible section headers gave no signal when open, the panel-header trace ID was nearly invisible, the observation tree's error color was a raw palette value that didn't adapt to dark mode, and the per-row copy button was an oversized, over-rounded icon button whose click also opened the row's detail panel.

## Design

- **User column sizing.** `TraceUserIdentity` routes Slack users through `SlackUserIdentity` (already `text-body-sm`) and resolved users through `UserBadge`, which inherited the larger base size from `IdentityBadge`'s label. Passing `text-body-sm` to that `UserBadge` instance aligns both identity paths with the surrounding row text. The size cascades because the label span carries color but no size of its own.
- **Section headers.** `ContentSection` (Input / Output / Metadata) headers now take a subtle `bg-muted/30` fill while open, separating the open state from closed without competing with the hover fill.
- **Panel-header trace ID.** Moved from `text-[10px]` at 40% opacity to `text-[11px]` at full `text-muted-foreground` for legibility.
- **Observation tree error tone.** Now uses the semantic `--error` token instead of the raw `--color-coral-600`, so errored nodes (icon, stat values, waterfall bar) adapt to dark mode like the rest of the error UI. The generation-icon pink is unchanged.
- **Copy button.** `CopyButton` now stops click propagation, so copying no longer triggers a parent row/card handler — the traces table previously opened the detail panel on copy. The table's copy button is sized via `className` (`size-6`, `rounded-sm`, `size-3` icon), matching how the component is sized at its other call sites. The propagation guard is a benign default for those call sites (`LogViewer`, blueprint detail, knowledge store detail), which are not inside clickable containers.

## Migration

No action required.
