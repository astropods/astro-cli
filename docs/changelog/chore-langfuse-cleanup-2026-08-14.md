# Remove the legacy dataset verdict filter

## Summary

Dataset judgments use the database as their source of truth, not Langfuse metadata. The single Good/Bad verdict is a legacy construct: the UI dropped its verdict filter and pill, so the endpoint and Langfuse storage that backed it are removed too.

## Design

New Langfuse dataset items omit verdict and confidence metadata; only criteria-related judgment metadata is written to Langfuse. `GET /deployments/:id/dataset/items` no longer accepts `verdict` or `cursor` query params and no longer returns `next_cursor`; it lists items with plain page-based pagination. This removes the handler logic that scanned successive Langfuse pages to reconstruct a filtered, cursor-paginated view. Per-criteria good/bad counts (`DatasetGradeSidebar`) are unaffected: they come from the database, not this verdict metadata.

## Migration

No migration is required. Existing Langfuse metadata remains compatible.
