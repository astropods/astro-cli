# Prediction request storage

## Summary

Eval dataset predictions now have durable request state so asynchronous generation can be observed and safely retried.

## Design

One current-state row per dataset trace records queued, in-progress, completed, or failed generation. Completed prediction output remains in the existing prediction tables. Requeueing terminal work reuses the request row, clears its failure message, and preserves active work.

## Migration

No manual action is required. The schema change is additive.
