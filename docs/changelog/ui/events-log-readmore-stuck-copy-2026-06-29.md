# Events log: readable long messages + clearer stuck copy

## Summary

Two small deployment-events UX fixes. Long Kubernetes event messages (e.g. a
`FailedScheduling` detail) had no way to be read in full once they overflowed,
and the "stuck deployment" headline buried the call to action. This makes long
messages expandable and leads the stuck-event title with the action.

## Design

**Expandable event messages.** The events tab message is now wrapped in a small
`EventMessage` component (in `PodDetailPanel.tsx`). It clamps to two lines and
measures the rendered paragraph; a "Show more" / "Show less" toggle appears only
when the text actually overflows the clamp, so short messages stay clean and any
length of error is fully readable on demand. This replaces the previous split
behavior (warnings rendered full-height, normal events clamped with no way to
expand).

**Stuck-deployment copy.** The server-side humanized title for `FailedScheduling`
changes from `Deployment stuck — needs action` to `Action required. Deployment
stuck`, leading with the action. The title is produced by
`humanizeDeploymentEvent` in the server and rendered verbatim by the client, so
the copy lives in one place.

## Migration

None. UI/copy only.
