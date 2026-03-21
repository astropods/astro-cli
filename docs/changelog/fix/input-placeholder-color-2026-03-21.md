# Fix: Input placeholder text color

## Summary

Placeholder text in `Input` and `Textarea` components was using `muted-foreground`, which is intended for secondary content. Placeholder text should be visibly dimmer to distinguish hint copy from real input values.

## Design

Placeholder text now uses `faint-foreground` across both components. The `faint-foreground` token was also recalibrated to create a proper three-tier contrast hierarchy:

- `foreground` — primary text (~19:1 light, high contrast dark)
- `muted-foreground` — secondary/supporting text (~9.7:1 light)
- `faint-foreground` — placeholder/hint text (~4.6:1 light)

Light mode: stone-600 → stone-500. Dark mode: stone-600 (same as muted) → stone-700 (correctly dimmer).

## Migration

No changes required. Token consumers that already use `faint-foreground` will see slightly adjusted values.
