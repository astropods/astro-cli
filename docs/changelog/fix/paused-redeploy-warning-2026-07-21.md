# Let paused agents redeploy, with a reactivation warning

## Summary

A recent change blocked Redeploy on the Configure page while an agent was paused and offered a "Resume to deploy" button instead. Review feedback pointed out this is the wrong default: someone often pauses an agent precisely to take it out of traffic (for example, it has a bug), and forcing them to resume first is backwards. There is also no backend requirement for it: the deploy path never checks paused state.

## Design

Redeploying a paused agent already reactivates it. The deploy handler resets the deployment record to pending and reapplies the full spec (including the normal replica count), so the agent comes back up and serves traffic again regardless of its prior paused state. The old UI guard hid that behavior behind a hard block.

The Configure footer now allows Redeploy (and Save, and Discard) while paused, and instead surfaces the consequence:

> This agent is paused. Redeploying will reactivate it and resume serving traffic.

The warning shows only for actions that actually redeploy. A name-only Save uses the rename endpoint, which does not deploy or change status, so a paused agent renamed stays paused and no warning is shown.

## Migration

None. Behavior-only change on the Configure page footer.
