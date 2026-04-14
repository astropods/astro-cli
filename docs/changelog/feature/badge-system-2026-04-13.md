---
title: Badge system — StatusBadge and Tag primitives
---

## Summary

Replaces scattered inline `style` overrides on `InlineBadge` with two purpose-built primitives: `StatusBadge` for semantic status states and `Tag` for flat categorization labels. Domain-specific wrappers own the label and color mapping so callsites stay clean.

## Design

**`StatusBadge`** is a pill-shaped badge with a colored background, border, and foreground derived from a semantic color variant: `success`, `warning`, `error`, or `muted`. It accepts optional `indicator` (dot) and `spinning` props for live status states.

**`Tag`** is a flat, borderless label chip with squarer edges and named color variants: `default`, `teal`, `blue`, `yellow`, `coral`. Intended for categorization — privacy, deploy type, role — rather than status.

**Domain wrappers** sit between the primitives and callsites:

- `DeploymentStatusBadge` — rewired internally to use `StatusBadge`. All surfaces that already use this component auto-update.
- `HistoryStatusBadge` — new wrapper mapping `DeployHistoryStatus` to `StatusBadge`. Replaces inline style maps in `DeploymentHistoryRow` and `BuildHistoryGroup`. Exports `HISTORY_STATUS_FG` for the inset box-shadow color on the current deployment row.
- `TraceStatusBadge` — new wrapper mapping trace status (success/error/timeout) to `StatusBadge`. Replaces inline style maps in `MonitorTab` and `TraceDetailPanel`.

**Callsite migrations** — `PrivacyBadge` now renders `Tag` (default color) inside its tooltip wrapper. Organization role badges in `OrganizationsSettings` use `Tag` (admin → teal, member → default).

## Migration

No changes required.
