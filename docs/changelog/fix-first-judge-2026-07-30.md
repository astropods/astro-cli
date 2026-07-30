# Retry concurrent judge key provisioning

## Summary

Concurrent first-time prediction jobs can briefly observe an upstream judge key before its local key record has been saved. This transient state now retries instead of permanently failing the prediction.

## Design

The eval judge worker leaves the prediction request in progress and delegates retry scheduling to River. A later attempt reuses the local key record saved by the winning job. If no local record appears after all attempts, the request is marked failed as a genuine orphaned-key condition. Key decryption failures remain immediately terminal.

## Migration

No migration or user action is required.
