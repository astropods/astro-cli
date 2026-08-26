# Drop the "Errors in logs" banner from the pod detail panel

## Summary

Opening a pod put an "Errors in logs" panel above the tabs, holding the first
error found in that pod's logs. On a pod with a stack trace in its output the
banner ran to a dozen lines and pushed the container diagnostics on General off
the first screen.

The error is not lost. It is in the Logs tab of the same panel, in context and
with the lines around it, which is where someone reading a stack trace wants to
be anyway.

## Design

The banner was the only consumer of the log-error probe inside the panel, so the
probe goes with it: `useContainerErrors`, the per-container
`ContainerLogErrorProbe` fan-out, and the `firstContainerError` lookup. Opening a
pod no longer fetches every container's logs to decide whether to warn about
them.

`PodTile` keeps all three. Its error dot is a different affordance, on a surface
with no room to show the message, so there the probe is what makes the state
visible at all.

## Migration

None.
