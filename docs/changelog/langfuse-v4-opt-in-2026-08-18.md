# Langfuse v4 reads are opt-in

## Summary

`LANGFUSE_USE_V4_API` flips from an opt-out to an opt-in. `astro-server` reads
Langfuse through `v3Reader` unless an environment sets the variable to an
explicit true value.

## Design

`NewClient` still selects one `traceReader` per process, so the choice stays a
single constructor branch rather than a per-call check. Only the direction of
the default changes: `useV4API` returns false when the variable is unset,
empty, or unparseable, and `true`, `1`, or `TRUE` select `v4Reader`.

The opt-in direction matches how the write side rolls out. An environment whose
Langfuse writes in `legacy` mode has no populated v4 ClickHouse tables, and the
v4 endpoints answer a miss with `200` and an empty list, so a v4 read there
surfaces as empty traces and empty metrics instead of an error. With an opt-in
default, an environment that has not cut over reads correctly without carrying
config, and a typo in the variable leaves it on v3 rather than pointing it at
tables nothing has written.

## Migration

Set `LANGFUSE_USE_V4_API=true` on `astro-server` in every environment already
reading v4. Environments still on `legacy` write mode need no config.
