# Per-container environment variables in pod detail panel

## Summary

The pod detail panel combined environment variables from all containers into a
single flat table, silently deduplicating by name. For multi-container pods this
was misleading — variables from different containers were mixed together and
container-specific values could be hidden.

## Design

`GeneralTab` in `PodDetailPanel` now groups environment variables by container
rather than flattening them into a single `Map`. Each container produces its own
bordered table with alphabetically sorted entries. When a pod has more than one
container a container-name heading appears above each table; single-container
pods omit the heading so the UI stays clean.

## Migration

None required.
