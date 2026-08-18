# Accurate agent name validation errors

## Summary

When an agent name was rejected, the API returned a hardcoded message ("must be
lowercase alphanumeric and hyphens only, 1-63 characters") that did not match the
rules the validator actually enforces. It understated the minimum length (4, not
1), omitted the start/end rules, and never explained reserved names, so a user
whose name was rejected for one reason saw an explanation for a different one.

## Design

The two server call sites (register agent, deploy source agent) no longer format
their own description. They call `spec.ValidateName` and pass its error straight
through, so the message always states the rule that actually failed (too short,
reserved name, or the character/start/end rule) and stays in sync with the
validator by construction.

## Migration

None. Error text only; the accepted set of names is unchanged.
