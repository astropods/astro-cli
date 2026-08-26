# Free credits modal for a new account

## Summary

A new account received signup credits and was told nothing about them. The modal that existed for this had no trigger and could show a hardcoded dollar amount rather than the account's actual grant.

This branch targets `feat/billing-balances-endpoint`, which adds `GET /billing/balances`: the amount this modal shows comes from it, since it's the only server-side read that carries a credit's actual grant rather than just what's left. See that branch's changelog for the endpoint's own design.

## Design

The amount comes from the account's real credit grant via `useBillingBalances`, reading what was granted rather than what's left, and only for a grant denominated in USD. A grant in another unit has no dollar figure to announce, and an account with no grant renders nothing rather than a fabricated number. Metronome's package configuration stays the only source of truth for the amount.

The trigger is a one-time, per-user localStorage flag set by `Onboarding.tsx` right after account creation, the same one-time-per-user pattern as the existing `usePersistentCoachmark` but inverted: presence means "show it". It clears the first time the modal closes, so returning by any other route never reopens it. `?freeTrial=1` remains as a manual QA override.

`FreeTrialModalHost` mounts app-wide in `Layout`, so it reads the flag before anything else and keeps the balances query in a child that renders only when there's something to open. Sessions that will never see the modal issue no request for it.

The card is ported from the design mock onto the app's Dialog primitive, over the existing `StarField` rather than a second canvas renderer. Below the `useIsMobile` breakpoint it renders as a full-width bottom sheet, matching what `SidePanel` does for its own overlay content. It's one fixed dark illustration in both themes, so its colors are literals rather than semantic tokens.

`useBillingBalances`, `getBillingBalances`, and the typed `BillingRecord`/`BillingBalances` shapes are the only pieces carried over from an earlier, separate credit-ledger UI effort; that UI itself (a Credits & Commits tab) was superseded by `feat/billing-and-usage-ui-update`'s single-page Billing redesign and isn't part of this branch. `toBalanceRow` (reading a raw credit record into the fields this modal and any future ledger UI would both want: granted, remaining, expiry) comes along with it, since the modal reads `granted` through it.

**A balances read that finds nothing doesn't mean there's no grant.** Signup credit is granted off the request path (a queued job, `handlers/accounts.go`), so the balances query firing the moment the account is created can easily see nothing yet, before an error and before the credit actually lands. `PendingFreeTrialModal` used to treat "loaded, no USD grant found" as proof there's genuinely none, and cleared the one-shot pending flag on that alone, permanently losing the announcement to either a transient failure or ordinary provisioning lag. It now only counts a *successful* response with *no* error, or a *persistent* error, as evidence, and even then retries up to `NO_GRANT_MAX_RETRIES` times, three seconds apart, before concluding there's truly no grant and clearing the flag.

An error and an empty success share the same bounded retry window rather than the error being unclearable forever: `useBillingBalances` already absorbs a merely transient failure with its own internal retries before ever reporting `isError`, so by the time this component sees one, treating it the same as an empty response doesn't reintroduce the risk of losing the announcement to a blip, it just also resolves the case where the request keeps failing past that window, instead of leaving the flag stuck and re-querying on every page load indefinitely.

**Public docs.** `usage-limits.mdx` gets a short "Signup credit" section: what it is, that it's shown once, and that there's no page to view it again yet (linking to Usage for the period's spend total instead). This is the branch that actually ships the user-visible surface, so docs land here rather than being deferred again.

**Raw color utilities now carry their `dark:` pairing.** The card's CTA button used `bg-indigo-600`/`hover:bg-indigo-500` with no `dark:` variant; a later pass found the same gap on every other raw-white utility in the file (`text-white`, `text-white/60`, `text-white/70`, `text-white/80`). All now pair with an identical `dark:` variant, matching the file's own stated design: one fixed dark illustration in both themes, not a light-mode default that dark mode overrides.

**The CTA now actually deploys an agent.** `onCta` was wired to the same handler as the X button: clicking "Deploy an agent" only dismissed the card. It now also navigates to the account's `blueprintsAccountPath` before closing, so the modal's one conversion moment leads somewhere instead of just disappearing.

**Static typography moved out of `style={}`.** Font family, size, weight, line height, and letter spacing on the badge and balance figure are now Tailwind classes (`font-mono`, `text-[11px]`, `tracking-[.16em]`, etc.), matching `astro-client/CLAUDE.md`'s "never use `style={}`" convention for values Tailwind can express. The gradients, shadows, and rgba() colors stay inline: this codebase has no existing use of a multi-stop `bg-[...]` gradient or a `textShadow`/`boxShadow` arbitrary value, so converting those here would be untested against real rendering rather than following a proven pattern, and the `cubic-bezier()`/`color-mix()` animation values were already a documented exception (the comma inside those function calls breaks Tailwind's arbitrary-value parsing).

**Docs style.** `usage-limits.mdx`'s new section used bare "Astro" instead of "Astro AI", and "There's no page to view it again yet" read as a roadmap promise. Both fixed.

**The retry loop no longer wastes retries on an answer that already arrived.** `unresolved` treated "loaded, no *USD* grant" the same as "loaded, nothing at all," so a grant that had already landed in a non-USD unit (the "Token grant" test case) still burned all `NO_GRANT_MAX_RETRIES` refetches over 15s before the flag cleared, even though the first response already contained everything Metronome had. A response with at least one credit row, just not a qualifying one, is resolved on arrival now; only a response with no rows at all, or a failed request, still gets the bounded retry window, since either of those could still change once the signup credit's queued provisioning job finishes.

**Dropped the `console.warn` on the no-grant path.** It fired on ordinary outcomes, not just bugs: a non-USD grant, the OSS/noop provider (which always reports `available: false`), or a provisioning job that simply ran past the retry window. Every other `console.warn` in this codebase marks a genuine caller-invariant violation; this one didn't belong in that company, so it's gone rather than gated behind a dev flag this codebase has no existing pattern for.

**The raw-provider-fields question was already settled.** `feat/billing-balances-endpoint`'s changelog already records the decision to keep `Balances`'s pass-through as-is (including fields like `created_by`), consistent with `Invoices`/`UsageData` doing the same, confirmed rather than left silent. No new decision needed here.

## Migration

None. Existing accounts never had the flag set, so the modal appears only for accounts created from here on.
