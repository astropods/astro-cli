## Summary

Adds a focused verdict animation to the Eval review queue so reviewers can see each Good, Bad, or Neutral judgment travel toward the live grade summary.

## Design

The Eval page exposes the grade badge as an explicit animation target and passes that target into the review queue. Verdict buttons provide their clicked element as the animation origin, and the queue launches a small verdict token toward the grade badge before pulsing the target. The motion follows the prototype's arc-and-pulse treatment while staying scoped to the review queue and respecting reduced-motion preferences.

## Migration

No user action is required.
