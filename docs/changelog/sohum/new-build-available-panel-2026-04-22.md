## Summary

The "new build available" banner on the agent detail page was opaque — it gave no indication of what had actually changed, making it hard to trust. This change enriches the banner with build comparison info and fixes icon alignment across all status panel variants.

## Design

The `ActionPanel` in `status-panel.tsx` now accepts an optional `children` prop rendered below the header row, indented to align with the title text. The header row switches to `items-center` so the icon, title, and button all vertically center together — previously `items-start` caused the button (taller than one line) to visually drift from the icon. The same `items-center` fix is applied to `BasePanel` (Error, Warning, Info, Success, Neutral).

In `ActiveDetailView`, the `latestBuildId` derivation is expanded to retain the full `latestVersion` object and also find `currentVersion` by matching the deployed `build_id` against blueprint versions. The banner now reads:

```
A new build number is available for this agent.
Current:   Apr 15, 2026 / 0b3cfd8b
New:       Apr 22, 2026 / d4763698
```

Build hashes are `font-mono font-medium`; dates are regular weight. The Storybook `ActionPanel` story is updated to reflect the new layout.

## Migration

Nothing required.
