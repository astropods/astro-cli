## Summary

The deploy form showed API and validation errors at the top of the page, far from the action that triggered them. This meant users had to scroll back up to read the error after hitting Save — especially disorienting on long forms with many configuration sections.

## Design

The error panel is moved to the bottom of `DeployFormFields`, after all form sections. The fixed action bar (Cancel / Save) sits at the very bottom of the viewport. By placing the error last in the scroll flow, it appears directly above the action bar when an error occurs. The existing `useEffect` in `ConfigureDeployment` already scrolls to the bottom on `deployError`, so the user sees the error immediately without any additional scroll logic.

The `ErrorPanel` component (and all tones in `status-panel.tsx`) was using `var(--color-red-700)` — a raw palette token that doesn't adapt to dark mode. Updated to `var(--error)`, the semantic token already used by `StatusBadge` for its error state, ensuring consistent color in both light and dark themes.

## Migration

No action required.
