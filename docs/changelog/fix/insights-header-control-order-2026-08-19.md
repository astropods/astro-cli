## Summary

The Insights header now presents the resolved date window and the range picker
as a single control:

```
┌─────────────────────────────────────┐
│ May 10 – Jun 8   7D 14D [30D] 90D   │   [ Sources ▾ ]
└─────────────────────────────────────┘
```

Before, the two were separate. The date label sat on the far side of the Sources
filter from the chips that set it, so nothing connected the range a user picked
to the window it resolved to. The label also vanished on narrow layouts.

## Design

**Control order.** The row reads date readout, range chips, Sources filter. The
Sources filter moves to the end, where it reads as a concern separate from time
selection.

**One control, not two.** Proximity alone still left the readout looking like
loose page text. The readout now sits inside the picker's own track, ahead of the
chips. The dates therefore present as the picker's output.

**Color separates the two halves, not a rule.** The readout takes the darkest
text step and the options drop one below their usual weight. The active option
still reads as active because it keeps its raised chip. A divider would add a
third vertical line to a header that already has two control borders.

**A `leading` slot and an `lg` size on `PillToggle`.** The slot renders a
non-interactive span as the track's first child, sized from the same per-size
table that sizes the chips. A readout is not an option, so it carries no button
role and no `aria-pressed`. The `lg` size puts the track on the 32px form-control
height, so the pill lines up with the Sources filter beside it.

Both are opt-in, and the color step-down applies only when a readout is present.
The pill keeps its compact 26px track and standard contrast at its five other
call sites, where it sits beside body text rather than in a row of controls.

**Visible at every width.** The dates are the picker's output, so a layout that
drops them leaves a picker that will not say which range it resolved to. The
control measures 262px at its widest, which fits the narrowest page area the app
renders, and `whitespace-nowrap` holds it to one line. The header row wraps
instead: the Sources filter moves to its own line when the two stop fitting side
by side.

## Migration

No action required.
