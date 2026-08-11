# Billing operations in queen

## Summary

Answering "why is this account not billing correctly?" meant four tools: the
accounts table for the customer IDs, `river.river_job` for whether provisioning
ran, the Metronome dashboard for the contract, and Stripe for the card. Three of
those hold half the answer each, and the join only exists in an operator's head.

Queen's account page now shows that join, and offers the two repairs that have
no vendor equivalent.

## Design

**The panel shows what only astro holds.** Our status machine and its reason,
whether enforcement acts or only observes, whether workloads are actually
stopped, when provisioning completed, and the state of the last provisioning
job with its error. None of this exists in Metronome or Stripe.

**Contract coverage is the provisioner's verdict, not a second opinion.**
`ContractCoverage` on the provider seam runs the same list-and-classify the
provisioning path runs, and both derive the uniqueness key from one function.
The panel therefore reports `ours`, `foreign`, or `none` exactly as
provisioning decided, and shows the contracts it decided from. A `foreign`
verdict is the one an operator has to clear, so it names the blocking contract
and links to it.

A provider that cannot answer reports `unknown` rather than `none`, because
`none` reads as "safe to provision".

**The numbers that explain the state are shown; the operations are not.** The
panel leads with credit remaining, the open period's draft total, and the last
finalized invoice. Credit remaining is the input to the gating decision this
system makes, so reading it should not require opening another tool.

Each amount carries a presence flag rather than being inferred from being
non-zero. An exhausted account and a failed lookup both produce zero, and only
one of them is a fact worth rendering.

**The money operations stay in the vendor dashboards.**
Archiving a contract, granting credit, and refunding already exist there with
their own permissions and audit trail. Rebuilding them here would duplicate that
and add a second way to break billing. The Metronome and Stripe customer IDs on
the account page link straight to them instead. Both links are environment-aware,
because the failure mode is silent: the wrong Metronome environment or the live
Stripe dashboard against a test key both report no such customer, which reads as
missing data rather than a wrong link. The Stripe link follows the publishable
key prefix; the Metronome one follows `METRONOME_DASHBOARD_ENV`.

**Two write actions, both ours alone.**

`RetryBillingProvision` re-enqueues provisioning. Today that means hand-written
SQL against `river.river_job`, because a blocked job is cancelled and the hourly
sweep is the only other path back. It refuses an already-provisioned account
rather than risk a second signup credit.

`ForceBillingResume` restores billing-suspended deployments. The resume worker
only touches deployments billing itself stopped, so it cannot start something a
customer stopped. It deliberately does not clear billing status: this unblocks
an account now, it does not decide the account should not be billed, so a still
unpaid account can be suspended again by the next signal.

Both write an audit row.

**One failing source degrades the view rather than emptying it.** Metronome and
Stripe reads are reported as warnings, so a vendor outage still leaves the
astro-side half readable, which is the half that says whether provisioning ever
completed.

That holds inside a source too. Spend is three separate reads, and a failure in
one is reported alongside whatever the others returned rather than instead of
it, so a slow invoice endpoint cannot hide the credit balance. The presence
flags decide what renders. The exception is a credit page that fails part way:
an incomplete sum understates the balance, which reads as closer to exhaustion
than the account is, so it is dropped rather than shown.

The panel is a separate RPC from `GetAccount`, so the account page renders
without waiting on two vendor APIs.

## Migration

Nothing to configure. The panel appears on every account detail page; a server
with no billing or payment provider shows coverage `unknown` and omits the card
rather than failing.

`METRONOME_DASHBOARD_ENV` names the Metronome environment the deep links point
at, and is set to `sandbox` for preview. Metronome dashboard URLs carry the
environment as a path segment (`app.metronome.com/sandbox/customers/<id>`), and
the API token is scoped to one environment, so this must name the same one the
token belongs to. Left unset, links use the default environment.
