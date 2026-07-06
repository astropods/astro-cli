## Summary

The chat details panel needs lightweight usage data without calling heavier detail APIs for every render. This change extends the existing observability summary cache so chat can reuse the same low-cost data path as the rest of the product.

## Design

Deployment summaries now include spend totals and daily spend series alongside request and token series. The River refresh job writes those values into the cached summary entry, and the bulk summary endpoint returns them with the existing trace-count data.

## Migration

No user action is required. Cached deployment summaries refresh through the existing background job, and clients that do not read the new spend fields continue to work.
