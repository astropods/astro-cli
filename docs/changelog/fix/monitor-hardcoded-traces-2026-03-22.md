## Summary

This hotfix removes development-only hardcoded trace rows from the Monitor view and restores the intended production empty-state behavior. It also keeps the observability notice dismiss interaction so users can clear transient warnings without layout regressions.

## Design

The traces table now renders only API-backed data and no longer falls back to generated mock rows when no traces are available. Empty and loading states use a constrained container height to prevent unwanted page-level scrollbars, and the observability banner includes a dismiss control that preserves compact spacing after dismissal.

## Migration

No migration is required.
