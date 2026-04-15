# Deploy page compute limit polish

## Summary

Improved the deploy page experience when an account has reached its compute unit hour limit. Previously the error was only surfaced after attempting to deploy, displayed with a distorted icon and unpunctuated copy. This change makes the limit visible immediately on form load and reduces friction for requesting a quota increase.

## Design

**Status panel icon fix:** `BasePanel` was missing `shrink-0` on its icon, allowing flex to compress the SVG width while the height attribute held — producing a distorted aspect ratio. `ActionPanel` had the same `items-center` alignment issue causing the icon to float mid-block on multi-line text. Both fixed with `shrink-0`, `items-start`, and a nudge via `mt-[1px]`.

**Compute limit UX:** `DeployFormFields` now calls `useAccountUsage` on mount and derives `isAtComputeLimit` from `compute_unit_hours.usage >= quota`. The banner renders at the top of the form (above General) whenever the limit is reached — before the user attempts to deploy. The "request a quota increase" CTA opens `RequestIncreaseDialog` in-context rather than navigating away to Settings.

**Deploy button:** `DeployBlueprint` also checks the compute meter and sets `disabled={isAtComputeLimit}` on the submit button. A tooltip — "Compute limit reached for this billing period" — is shown on hover via a `span` wrapper (required because disabled buttons don't fire mouse events).

**Toast confirmation:** Sonner installed and mounted at the app root. `RequestIncreaseDialog` fires `toast.success("Quota increase requested")` on successful submission.

**Copy:** Error banner title/body rewritten for clarity and punctuation. Detection is a case-insensitive regex on the API error string so it degrades gracefully if the message changes.

## Migration

No migration required. `sonner` added as a client dependency.
