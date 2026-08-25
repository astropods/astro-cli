# Credits tab: one balance card, unit-safe amounts

## Summary

The billing page spread one question across three places: a plan card above the tabs said which billing mode an account was on, a bare table listed credits with no sense of how much was left, and spend controls sat in their own section governing numbers shown elsewhere. The table also decided whether an amount was cents or dollars by pattern-matching the provider's display label, the one field the provider is free to rename.

## Design

The tab is **Credits**, and commits are gone from the UI: astro-server has no product need to surface them, so nothing fetches, renders, or names them. The `commits` field stays on the `/billing/balances` response and its type, since removing it is a separate API change nobody asked for.

`PlanSummary` and the credits table are replaced by a `CreditBalanceCard` that answers the whole question in one place: which mode the account is on, how much is left, and what happens when it runs out. A `Sparkles` badge marks the modes nothing stops, so the bar's color stays semantic rather than decorative.

The mode is derived rather than read off the plan. `spend.plan` names the Metronome package the contract sits on, decided once at provisioning, so it cannot answer what an account can do today: the same `credit` package reads differently once the credit is spent, and differently again once a card is on file. The card combines three server signals into five states, unlimited, signup credit, pay as you go, card required, and stopped. Whether credit remains comes from `spend.credit_remaining`, which the provider scales by credit type id, rather than from the client's own total, which is null for a credit type it cannot read as money and would silently report every such account as paying as it goes. Whether the account is stopped is the server's `credits_exhausted` verdict, so an account that never had a credit is never described as having spent one, and an account with neither credit nor card is asked for one rather than told it is paying as it goes.

The bar follows the mode, because depletion and accumulation are different questions. On the signup credit it tracks granted against remaining. Once the credit is spent it tracks the period's usage against the account's own limit, or says so plainly when no limit is set, since there is nothing to plot a percentage against. The floating pin is the card's focal element rather than a caption: it carries the headline figure at heading size and names what that figure is measured against, so the cap an account set for itself reads on the same axis as the spend it caps. It floats over the point the figure describes: the boundary while credit remains, the current usage against a cap, the middle of the spent span once nothing is left, and the center with no arrow where there is no scale at all. Uncapped pay-as-you-go reads as a flat track rather than a bar pretending to measure something, and stays green until it approaches a cap the account set itself. The unlimited plan drops the visualization entirely: it is an internal account with no balance to draw, so one line states the mode and the tab ends there.

Each grant renders as a "used of granted" row under whatever name the provider gives it. Nothing is hardcoded to a category, so an account's real grants show and an account without them shows nothing invented.

Limits are one card and one Save. "Stop agents at" means the same thing in every row, so the total spend cap sits in the same list as the per-metric compute and AI Gateway caps, each labeled with its own unit. Money and quantities still go to their own endpoints, and a partial failure names the row that did not land. The last finalized invoice and the period's running spend moved to the balance card, where the figures belong beside the balance rather than beside the controls.

The card says what it can and can't do. A viewer without `org:manage` gets the limits read-only with a line saying so, rather than a Save that 403s. A crossed limit reads "Agents stopped by this limit", since that state has stopped every agent in the account. An exhausted balance reads "Credit remaining $0.00" and names the fallback, and the period line says "Billing period ends" once credits are gone rather than promising a reset that isn't coming.

Amounts arrive in the credit type's own unit, so `creditUnit()` keys on the type's id, the same `usd_cents` id astro-server uses for `/billing/spend` (`internal/billing/metronome/spend.go`), and falls back to the label only when no id arrives. A type that resolves to neither is not money: the row reports its own amount and label, while the bar and the total are omitted rather than stating dollars for tokens. `BillingRecord` is typed to the fields the UI reads, so a renamed provider field is a compile error instead of a silent "$0.00".

Two things outside the tab came with it. The invoice PDF viewer is now a download button, backed by a mutation rather than a query since it is a one-shot action on click, sharing `downloadBlob` with the deployment-file download. The Quotas tab drops its "Utilization" column, which read as a second, conflicting measure beside a raw usage/limit pair; the numbers stay.

The spend endpoint forwards `last_invoice_total`, `last_invoice_at`, and `has_last_invoice`. The provider already computed them from the customer's latest finalized invoice; the handler wasn't passing them on.

## Migration

None.
