# Hold the gating latch while another spend alert is still over

Review fixes on the customer-set spend controls.

## Summary

Spend controls are per account. A personal account and each organization are
separate provider customers, the alert's uniqueness key carries the customer id,
and reads are scoped by it, so one account's limit cannot gate another.

What can collide is two alerts measuring the *same* account. Both gate through
`alert_active`, and `SignalAlertResolved` cleared that latch unconditionally, so
whichever resolved first resumed an account the other still stopped. Raising your
own limit above current spend resolves your alert, and the account resumed even
while a second alert over the same customer was in alarm.

Only an alert created by hand in the provider dashboard produces that second
alert today, because an account-wide backstop applying to every customer at once
was removed. That is exactly how the first one came to exist, and the guard is
what stops a repeat from silently resuming a stopped account.

## Design

`CustomerSpendThresholds` now reports `OperatorSpendInAlarm`, set when a
`spend_threshold_reached` alert that is neither of the customer's own is over.
The list endpoint already returned it, so this reads a fact that was being
discarded.

On a resolved spend event the worker asks whether another spend alert over the
same customer is still in alarm, and holds the latch if so. Both directions
matter: a resolved operator alert can leave the customer's limit over, and a
resolved limit can leave the operator's alert over.

The question is asked about the *other* side only. The reader collapses every
alert that is not the customer's own into one boolean, so asking it about an
operator alert that just resolved counts that alert against itself. A list read
that still reported it in alarm would hold the latch, ack the event, and leave
the account suspended with nothing left to resume it: the sweep only re-enqueues
suspends. The resolved event is authoritative for the alert that fired, so the
worker trusts it and reads only the other side.

A backend with no spend controls has no reader, and answers false. That keeps
one alert and one latch behaving as it did, rather than stranding every gated
account behind a check that can never pass.

## Also fixed

**A partial save is now named.** Changing a threshold archives the old alert
before creating its replacement, so a failure can leave that control unset. The
writes run in a fixed order with the limit first, so a failure on the warning
leaves the cap intact, and the 502 says which control may now be unset and which
took effect. The previous map iteration made that a coin flip.

**One home for the alert names.** `astro:spend_warning` was declared in two
packages that do not import each other, with nothing tying them together. A
rename reaching only one side would have made warnings gate, silently, with
every test still green. Both now come from `internal/billing`.

**No stale flash, and no stale value either.** A write now seeds the query cache
with what it stored, so the fields keep reading through instead of holding the
text that was typed. Both matter: resetting to nothing showed the pre-save
numbers until the refetch landed, and keeping the typed text left "50.999" on
screen against a stored threshold of $51, which is a form disagreeing with the
threshold that actually fires.

**One account's edit cannot reach another's form.** Settings pages read the
organization from a route param, so moving between two accounts re-renders the
component rather than remounting it, and an edited number followed the owner to
the next account where the next save would write it. The form is keyed on the
account, which discards the edit at the boundary.

## Migration

None.
