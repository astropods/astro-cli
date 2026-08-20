# Label the units on the billing usage totals

## Summary

The "Totals this period" table on the billing page rendered every billable
metric in one unlabeled column, so two numbers in different units read as one
unit. `Compute Units` totals `cu_hours` and `LLM Usage` totals `cost_usd`, which
means a row like `3.92` is a quantity of compute while `5.55` beside it is US
dollars.

The dollar row compounded the confusion. The gateway rate is 100 cents per unit,
a one-to-one pass-through, so that total happens to equal what the account is
charged for model usage. The compute total does not: 3.92 CU-hours bills at 6
cents each, or 24 cents. A reader who took the column as money read the compute
row as sixteen times its charge.

## Design

**The unit comes from the metric, not the column.** Each total now renders with
its own unit, money as money and a quantity with its unit named. A metric the
client does not recognise renders as a bare number rather than borrowing
another's unit, so a rename upstream degrades to the previous behavior instead of
labelling a number wrongly.

Neither total is the amount charged. The charge is the quantity times its rate,
which the Limits header already reports for the period. Naming the units is what
makes the two figures legibly different rather than contradictory.

## Migration

None. Display only.
