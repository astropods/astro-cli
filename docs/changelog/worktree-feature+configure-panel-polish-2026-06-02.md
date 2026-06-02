# Polish Advanced sizing panel in Configure

## Summary

The Advanced sizing panel in the Configure form had several rough edges: the slider thumb overlapped its tier labels, the cost number flipped between header and footer depending on collapse state, the storage slider stacked redundant copy when locked, and the layout broke at narrow side-panel widths.

## Design

The whole section is one bordered card with three stacked regions: a tinted header that doubles as the collapsible trigger, a tinted body that holds the sliders when open, and a persistent footer that always shows the cost breakdown alongside an estimated monthly total. Putting the cost in one location at all times — rather than shuffling it between header and footer — gave the user a single, stable anchor for the number.

The slider primitive itself was tuned to feel less harsh: the range/thumb now ease with a cubic-bezier transition, the thumb scales on `active`, and the disabled opacity drops further so locked controls read clearly. The tier-label strip got more breathing room (`mt-3`) so the thumb no longer collides with `25m`/`50m`/etc.

Disabled storage now shows a `Lock` icon next to the label and replaces the description with a single-sentence hint ("Disk size is locked after first deploy.") rather than stacking both lines.

The footer wraps and stacks below `sm` so the breakdown, divider, and total reflow cleanly when the side panel is dragged narrow.

## Migration

No migration required.
