# Disable the over-provisioned alerts

## Summary

The two right-sizing conditions, `cpu_over_provisioned` and
`memory_over_provisioned`, fire on healthy deployments. Both compare observed
usage against a fixed fraction of the reservation, so an agent that is simply
idle looks identical to one that is genuinely over-provisioned. The alert then
tells an owner to cut a reservation that may be correct for the agent's peak.

They are now disabled rather than deleted. The query and the copy are the
starting point for a threshold that accounts for how busy the agent is, so
throwing them away would mean rewriting both.

## Design

`Condition` gains a `Disabled` field, and `observation.ActiveConditions()`
returns the catalog minus the disabled rules. The evaluator sweep and the two
catalog surfaces (the deployment Alerts tab and the admin console's alert list)
read `ActiveConditions()`, so a disabled rule is neither evaluated nor shown.

`Conditions` keeps every rule, disabled included, and stays the lookup for
resolving a stored condition name to its title and severity. That split matters
for `catalogByName` in the admin service: a `deployment_alert_state` or mute row
written before the rule was disabled still renders with a title instead of a
blank cell.

The `info` severity and the `observation.info` Novu workflow stay in place. Both
now have no producer; keeping them means re-enabling either rule needs no
notification wiring.

## Migration

None. Existing `deployment_alert_state` rows for the two conditions no longer
resolve or clear, because the evaluator stops looking at them. They are inert:
nothing sends from them, and the admin list still renders them so an operator
can clear them. To drop them:

```sql
DELETE FROM deployment_alert_state
WHERE condition IN ('cpu_over_provisioned', 'memory_over_provisioned');
```

The `observation.info` preference toggle remains visible in notification
settings. It controls a workflow nothing triggers, so its setting has no effect
until a rule is re-enabled.
