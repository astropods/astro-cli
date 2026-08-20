# Gate an account that was provisioned without credit

## Summary

An account provisioned onto the no-credit plan could deploy and spend at the AI
gateway with neither credit nor a payment method behind it.

The gate already has the rule that covers this: `computeStatus` suspends when
credits are gone and no card is on file. The rule was unreachable. Its input,
`credits_exhausted`, is raised by exactly one thing, the provider webhook
`alerts.low_remaining_contract_credit_balance_reached`, and that alert fires when
a credit balance falls. An account granted no credit has no balance to fall, so
the alert can never fire and the latch is never set.

Provisioning then made it worse. It finished by applying `credits_granted` to
every account regardless of plan, which clears the latch. The one plan that
starts with nothing was told it had just been funded.

Signup credit belongs to a person, not an account, so the accounts affected are
the ones whose creator had already spent their single claim: a second personal
account, or an organization created by someone who already has one.

## Design

**The credit latch follows the plan.** Provisioning raises
`credits_exhausted` for the no-credit plan and clears it for the plans that hold
a balance. Nothing else changes: `computeStatus` already ranks the reason, the
deploy, wake, restart, rollback, chat, and ingestion paths already call the gate,
and the account still clears itself by adding a card through the existing
`card_added` signal.

**Unlimited is not latched.** It rates every metered product at zero, so it owes
nothing and has nothing to exhaust.

**A card still wins.** The suspension is conditional on having no payment method,
so an account that adds one bills pay as you go with the latch still set, exactly
as an account that spent its credit does.

## Migration

Accounts provisioned before this change keep whatever latch they have. Two
preview accounts hold no credit grant and no card and are not currently gated;
they need `credits_exhausted` set once. Every other existing account either holds
a balance or has already been through the alert.
