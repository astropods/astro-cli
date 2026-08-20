# Classification survives one failing day, batch, or prompt

## Summary

Prompt classification gave up an account's whole pass for any single fault. One
failed day discarded the days planned behind it, one failed classifier call
discarded the batches before it, and one prompt the predictor refused cost its
whole batch its labels. Inference is billed before results are stored, so each
of those threw away work the account had already paid for, and the next tick
paid for it again.

## Design

### A fault is scoped to the thing that faulted

The pass continues past a failed day, because one day's fault is rarely the next
day's. Langfuse rejecting the account's credentials is the exception: that fails
every day identically, so it stops the pass and logs once at error level.

Each axis now persists before the next one runs, and a classifier call returns
the batches that completed alongside the error from the one that did not. A
trace fetch that dies mid-page returns the pages it read, so the prompts already
fetched are labelled rather than fetched and inferred again next tick.

### Cursors record only contiguous covered ground

`classified_through` and `backfilled_from` bound a window the pass claims is
fully classified, so a day sealed inside one is a day no later tick revisits.
Each edge advances over the unbroken run of completed days behind it and stops
at the first day that failed, was capped by the tick budget, or was never
reached. Days completed after a gap are still stored, they just do not move a
cursor over the gap. This also closes a hole in the budget path, which used to
leave the forward cursor claiming days it never processed.

Backoff follows the same split. A pass that covered nothing arms it, because
that pass spends the same inference again next tick for the same nothing. A pass
that moved records its fault and keeps its hourly cadence, so one stuck backfill
day cannot throttle the forward edge.

### Retry what a later attempt fixes, split what it does not

The classifier client separates a predictor that is unavailable from one that
rejected the input. An unavailable predictor is retried three times with a
widening wait, which covers a pod rolling under the call. A rejected input is
never retried, because the answer does not change: the batch is halved until the
refusal is attributable to a single prompt, which comes back unlabelled and is
recorded under the axis fallback. A batch refused as a whole stops the call
instead, so a broken predictor cannot label a day out of blanks and cannot be
asked again once per split of every batch behind it.

`langfuse.IsAuthFailure` replaces the copy of that predicate in the insights
roll-up worker, so both pipelines agree on what a rejected credential looks like.

## Migration

None. No configuration changes and no API changes. Days that failed earlier
retry, because a failed day stays outside the window.
