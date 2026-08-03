# Actionable image-pull remediation copy

## Summary

When a deployment event is an image-pull failure, the humanized guidance told users to check the image name, tag, and registry credentials. None of those are things a user manages on Astro, so the advice was a dead end. This reword points to the actions users can actually take. It also stops a generic `Failed` event that merely names an image from being mislabeled as a stuck image-pull failure.

## Design

Event humanizing lives in `HumanizeEvent` (`internal/deploycontroller/events.go`), which maps raw Kubernetes event reasons to a user-facing title, guidance, and severity. Two changes:

- The stuck image-pull guidance now reads "The build's image couldn't be pulled from the registry. Push a new build with ast push, or trigger a rebuild if this agent deploys from GitHub, then redeploy."
- The generic `Failed` reason is surfaced as an image-pull failure only when the message mentions pulling. Keying off the bare word "image" mislabeled unrelated failures that happen to name an image as a stuck "Action required: Image pull failed". The transient `BackOff` handling is left as-is.

## Migration

None. Copy and classification change only.
