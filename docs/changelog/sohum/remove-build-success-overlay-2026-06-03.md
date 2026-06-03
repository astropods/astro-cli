# Remove blueprint build-success overlay

## Summary

The onboarding wizard's draft→published transition triggered a full-screen "Blueprint is live" overlay (icon + scan line + confetti + `View blueprint →` button) on the GitHub path. The overlay obscured the page the user had just arrived at and several users reported hitting it as a confusing extra step after "Finish setup" already resolved. Removed.

## Design

`BlueprintDetail.tsx` no longer tracks the prior draft state or renders an overlay on the draft→published edge. The blueprint page renders directly with the build rows and identity already visible — no intermediate celebration screen.

The `LiveRevealConfetti` primitive stays in the codebase; it's still used by `LiveRevealOverlay`, `NewBlueprint`, and the knowledge-store onboarding flow.

## Migration

None.
