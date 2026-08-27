# Free trial modal: standard components, real fixes, conditional CTA

## Summary

The free trial modal shipped as a mostly hand-rolled port of an external
design: its own Radix dialog wiring, a raw `<button>`, a bespoke pill
`<span>`, and a custom blurred overlay, none of it built on the primitives
the rest of the app already uses for the same job. That divergence caused
two real, reported visual bugs, and its "Deploy an agent" CTA always sent
every account to `/blueprints` even when the account had none to deploy.

## Design

**Rebuilt on the shared primitives instead of alongside them.** The modal
now composes `Dialog`/`DialogPortal`/`DialogOverlay`/`DialogClose` from
`@/components/ui/dialog` (the same building blocks every other dialog in
the app uses), `Button` for the CTA, and `InlineBadge` for the "Free
credits on us" pill, layering the card's own dark, starfield-backed look on
top via `className`/`style` rather than reimplementing dialog plumbing.
`--primary`/`--primary-foreground` resolve to indigo/white in both app
themes, so switching the CTA to `Button`'s default variant is visually a
no-op, not a redesign. The full-viewport centered layout stays custom
(`DialogPrimitive.Content` directly, not the standard form-dialog
`DialogContent` box), since that positioning genuinely differs from a
form dialog rather than being unaddressed debt.

**Overlay blur fix.** The desktop dialog layered `backdrop-blur-md` and a
custom radial-gradient scrim on top of the dialog primitive's overlay,
producing a heavier blur than any other modal in the app. Every other
dialog and sheet uses a flat `bg-black/50` with no blur; the free trial
modal now reuses that same `DialogOverlay` unmodified.

**Bottom-edge hairline fix.** The card used a real CSS `border` on an
`overflow-hidden`, fully-rounded box. That combination can leave a
sub-pixel seam between the border stroke and the clipped background in
some browsers, most visible as a thin dark line at the bottom edge where
the gradient background is darkest. Replaced the border with an inset
`box-shadow` set that draws the same three-sided highlight (left, right,
bottom, matching the original's `border border-t-0`) inside the same
clipped box, so there's no separate layer to gap from. Not independently
verified against the original browsers reporting the bug, since it can't
be rendered in this environment; worth a visual check after this lands.

**Conditional CTA.** `FreeTrialModalHost` now reads `useAccountBlueprints`,
the same hook and `hasBlueprints` check `DashboardAgentsEmptyState` already
uses for its own CTA, and routes to `blueprintsAccountPath(account)` when
the account has any, `explorePath` otherwise. An account with zero
blueprints previously landed on an empty `/blueprints` page straight out of
the modal that's supposed to get them deploying something.

**Inline-style ratchet improved.** The pill's background/border were
literal `rgba()` values in `style={{}}`, one of them (`background`) already
flagged by `local-theme/no-static-inline-style`. Both are static, so both
move to Tailwind arbitrary-value classes (`bg-[rgba(...)]`,
`border-[rgba(...)]`) instead of `style`; the design still doesn't use
theme tokens here on purpose, since this card is deliberately dark in both
app themes. Lowered `check-inline-style-budget.mjs`'s baseline from 4 to 3
in this PR to lock the win in.

## Migration

No API or data changes. No behavior change for an account that already has
blueprints; an account with none now lands on `/explore` from the free
trial modal's CTA instead of an empty `/blueprints` page.
