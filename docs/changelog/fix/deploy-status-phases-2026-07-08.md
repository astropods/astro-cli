# feat: differentiate Building, Preparing, and Deploying on the deploy status card

## Summary

On the deploy page, the build-in-progress card (Preparing/Building) and the "Deploying" tile both used the same amber warning treatment, so a build card sitting above a deploying card looked off and the phases were hard to tell apart. This separates them by color. Closes #1569.

## Design

The deploy phase keeps its established amber treatment; the newer build phase moves to blue so the two read as distinct when stacked:

- **Deploy phase (amber):** the "Deploying" badge stays on the amber warning color with a spinner (unchanged), which reviewers preferred for the live deploy.
- **Build phase (blue):** the build-in-progress card ("Preparing" and "Building") moves off the shared amber onto a distinct blue with a spinner, so it no longer mirrors the deploying card it stacks above. The blue uses a new `--info` semantic token (blue-600 light, blue-400 dark) so the text stays legible in both themes.

The distinction is carried by color, so it holds across light and dark themes; the build sub-states remain distinguished by their label ("Preparing" vs "Building").

- **No stacked in-progress indicators.** The build and deploy phases are sequential (build runs, then deploy), so the build-in-progress card is suppressed while the deployment is actively deploying or undeploying. During a deploy the tile's status badge is the single in-progress indicator; the build card only appears in the pre-deploy window. This avoids two in-progress boxes stacking in the small deployment panel.

## Migration

None. This is a visual-only change.
