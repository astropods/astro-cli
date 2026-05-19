# Deprecating variables and surfacing them in the deploy UI

## Summary
Agent specs needed a way to mark a variable or sub-field as deprecated
and explain the migration path. The first concrete user is SLACK_CONFIG's
`allowed_user_ids`: user-ID gating is no longer enforced, but the field
still appears in the deploy form for existing deployments. Without a
deprecation signal, operators have no way to know it's safe to ignore.

## Design
A new `Deprecated string` (omitempty) on both `spec.Variable` and
`spec.VariableField` carries the migration message. Empty string means
"not deprecated" — no semantic change for existing specs.

`apps/astro-server/internal/deployment/template.go` sets it on
`allowed_user_ids` with: *"User-ID gating is no longer enforced.
Restrict access via allowed_channel_ids instead."*

In the deploy form (`VariableFields.tsx`), a deprecated field renders
with a strikethrough label, a "Deprecated" `InlineBadge`, and a tooltip
carrying the message. The "Optional" tag and description tooltip are
suppressed so the label row stays uncluttered. The input itself sits at
`opacity-60` with `focus-within:opacity-100` so the field is visually
demoted at rest but fully usable when the user focuses it to clear a
stale value.

## Migration
None. Existing specs keep working; deprecating a future field is a
one-line spec change.
