## Summary

Adds the backend API surface for building eval datasets from production traces. The endpoints let clients fetch dataset summary state, list judged dataset items, review an unjudged trace queue, and submit trace judgments.

## Design

Dataset reads stay deployment-scoped under `/deployments/:id/dataset`. The review queue endpoint fetches trace input/output from Langfuse, filters out locally judged trace IDs, annotates traces with the existing sentiment heuristic, and returns a paginated batch for review. Queue pagination uses `offset` within a fixed `end_time` snapshot so new traces do not shift the page window while a user reviews older results.

Judgment submission records the local judgment first as the duplicate gate. Good and bad judgments then write a deterministic Langfuse dataset item and bump local good/bad counters. If the local count update fails after the Langfuse write succeeds, the server deletes the deterministic Langfuse dataset item before releasing the local judgment row so retries start from a consistent state. Unknown judgments are recorded locally so the same trace is not re-surfaced, but they do not create Langfuse dataset items. Successful submissions return the local eval dataset ID, trace ID, and normalized verdict; clients can refetch dataset summary state separately.

The judgment endpoint publishes its JSON request body in OpenAPI so generic clients, including Queen, can render and forward the trace judgment payload.

## Migration

No user action required. The dataset summary continues to expose `item_count`, now derived from the good and bad judgment counts.
