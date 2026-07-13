## Summary

The deploy form showed the messaging adapter validation error as a padded alert while nearby field errors used plain inline text. That made one validation state look like a different class of problem even though it belonged to the same form submission.

## Design

Messaging adapter validation now renders as inline destructive text, matching required credential errors and other deploy form validation messages. The deploy form regression test covers the all-adapters-deselected path and verifies the message no longer renders as an alert block.

The messaging interface picker also marks the whole interface group as invalid when the agent requires at least one messaging surface and the user deselects every option. That border uses the same thin destructive treatment as missing credential fields, so the visual cue points at the exact group that needs attention without making one card look independently selected or broken. Once any interface is selected, the group-level invalid border clears.

## Migration

No user action is required.
