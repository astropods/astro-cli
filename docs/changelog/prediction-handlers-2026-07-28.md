# Dataset prediction enqueue API

## Summary

Clients can request asynchronous eval-dataset predictions for recent traces without waiting for model execution.

## Design

The bodyless deployment-scoped endpoint scans recent deployment traces and selects up to 50 that have input, no judgment, and no completed prediction. It batch-persists durable prediction requests and submits one independently retryable job per trace. River inserts are atomic and retain active-job uniqueness; a failed batch is recorded for retry by a later POST. The response reports trace IDs that were enqueued or failed to enqueue.

## Migration

No manual action is required.
