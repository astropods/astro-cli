# Fix false "Error" on starting pods and theme the status panels

## Summary

A pod whose container was still starting up rendered as "Error" with a red
"Starting up" line, which contradicted itself and looked like a failure during
normal startup. Separately, the deploy status and action panels drew some colors
from raw palette tokens, so they did not flip between light and dark themes.

## Design

The client marked any container in a "Waiting" or "Terminated" state as a
problem. That conflates a real failure (ImagePullBackOff, CrashLoopBackOff) with
a container that is still coming up: the server humanizes the benign startup
reasons (ContainerCreating, PodInitializing) to the message "Starting up", and
that container is also "Waiting". The problem check now treats a container as a
failure only when it is not that benign startup, so a starting pod reads as
"Starting" and its container card stays neutral instead of red. The shared
`isContainerProblem` helper drives both the pod tile and the container card.

The status/action panels now derive every tone from its semantic token
(`--info`, `--success`, `--warning`, `--error`) instead of raw palette values,
so background, border, text, and the primary button all flip with the theme. The
bright warning button takes dark text and the darker tones keep white, so the
button label stays readable in both themes.

## Migration

None. Presentation only, no API or data changes.
