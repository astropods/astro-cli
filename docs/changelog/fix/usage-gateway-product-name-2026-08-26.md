# Show gateway spend on the usage page

## Summary

The usage page reported `$0.00` of Models spend for every account, whatever the
account had actually spent on the gateway. Compute was correct on the same
screen.

The Compute and Models figures come from the daily-spend endpoint, which reads
Metronome's invoice breakdown and returns each day's dollars keyed by line item
under `by_product`. The client looked those keys up by billable metric name:
`Compute Units` and `LLM Usage`.

An invoice line item is named for the **product** that billed it, not for the
metric behind it. Compute's product and metric are both called `Compute Units`,
so that lookup hit. The gateway's metric is `LLM Usage` but its product is
`AI Gateway`, so that lookup missed and the stream fell back to zero. One stream
matching by coincidence is why the mismatch read as an empty gateway rather than
an empty page.

Confirmed against sandbox: for `postman-preview` the breakdown carries
`usage | AI Gateway | 1.09` alongside `usage | Compute Units | 47.81`, and
Metronome's product list gives the gateway product the name `AI Gateway` against
billable metric `LLM Usage`.

Nothing was mis-metered. `station bifrost reconcile` puts the same account at
100% captured, gateway ledger and Metronome agreeing to the cent. The spend was
billed, and only the display dropped it.

## Design

**Compute is matched by name; every other usage product is model spend.** The
period total for a closed period is the sum of the two streams, so a product that
matches neither name has to land in one of them or that total silently
understates itself. Folding the remainder into Models keeps the two figures
adding up to the total, and makes a future product visible in the wrong bucket
rather than invisible in none.

The product vocabulary is now its own constant, named to say that it is not the
metric vocabulary. The metric names stay where they belong, on the usage rows
that really are keyed by billable metric.

## Migration

None. Display only.
