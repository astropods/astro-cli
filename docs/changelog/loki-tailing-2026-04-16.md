# Extract log levels from Loki tail websocket

## Summary

The Loki tail websocket endpoint does not automatically promote structured metadata (like `detected_level`) into stream labels — unlike `query_range` which does. This meant live-tailed log lines always had an empty `Level` field, even when Loki had successfully auto-detected the level at ingestion time.

## Design

Loki 3.x stores `detected_level` as structured metadata. The `query_range` endpoint promotes it into stream labels automatically, but the tail websocket does not. The fix appends `| keep detected_level` to the LogQL query used by `TailLogs`, which explicitly tells Loki to surface that metadata in the stream label set.

The tail read loop now applies the same level priority cascade as `QueryLogs`:
1. Explicit `level` stream label (legacy pipelines)
2. `detected_level` from structured metadata (Loki 3.x)
3. `"unknown"` values discarded to empty string

This was verified against a live Loki instance — without `| keep`, the tail frames have no `detected_level`; with it, every stream includes the correct level.

## Migration

No migration required. The change is backward-compatible — `| keep` on a label that doesn't exist is a no-op.
