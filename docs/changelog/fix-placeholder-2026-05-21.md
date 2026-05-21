# GitHub Connect Modal: Branch / Path Field Order

## Summary

The path field in the GitHub connect modal appeared above the branch selector, which felt out of order — branch is a required, primary choice while path is an optional refinement. The path placeholder also looked like a real value rather than an example.

## Design

In `RepoPicker`, the branch selector now appears directly after the repository picker, and the path field follows below it. The path placeholder was updated from `services/my-agent` to `e.g. services/my-agent` to make clear it is an example.

## Migration

No action required.
