# Quick switcher stale after delete

## Summary

Deleted agents lingered in the agent-detail quick switcher for up to a minute
after the agents page had dropped them.

## Design

The switcher reads `deploymentKeys.summary`, a sibling of
`deploymentKeys.all(account)` — neither is a prefix of the other, so the
existing invalidate in `useUndeployAgent` didn't match. Added a second
invalidate on the summary key. Both delete entry points unmount the switcher
before it fires, so this just stales the cache; the next agent-detail mount
refetches.

## Migration

None.
