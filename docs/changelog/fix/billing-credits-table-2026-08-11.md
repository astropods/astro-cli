# Show credits and commits as a readable table

## Summary

The Credits & Commits tab rendered whatever the billing provider happened to
return. Columns were the union of scalar fields on the raw provider record, so
the table showed internal ids, `rate_type`, `priority`, and `created_by`, which
renders the name of the API key that granted the credit. It also overflowed
horizontally, and the fields an owner actually needs were nested objects and so
were dropped: the granted amount, the expiry, and the credit type.

The result was a wide table of internal detail with a bare `0` where the
remaining balance should be, and no indication that the 0 meant dollars.

## Design

**A projection, not a reflection.** `toBalanceRow` maps a provider record to
the four fields that mean something to an account owner: name, granted,
remaining, and expiry. Credits and commits share the provider's shape, so one
table serves both. Everything else the provider sends is dropped rather than
displayed, which also stops internal identifiers reaching a customer.

**Amounts are formatted in their credit type.** The provider reports in the
credit type's own unit, and ours is `USD (cents)`, so a $10 credit arrives as
`1000`. Amounts in a USD credit type are divided and rendered as currency; any
other credit type falls back to the raw number with its unit appended, so a
future non-USD type degrades to something truthful rather than a wrong dollar
figure.

**Zero is distinct from absent.** A spent credit has a real balance of 0 and
must render `$0.00`. Only a genuinely missing value renders an em dash. The
same distinction applies to a credit with no schedule, where the granted amount
is unknown rather than zero.

The projection lives in `lib/billing-balances.ts` rather than the component, so
it is unit tested directly and the component file keeps exporting only
components.

## Migration

None. Display only; no API or data changes.
