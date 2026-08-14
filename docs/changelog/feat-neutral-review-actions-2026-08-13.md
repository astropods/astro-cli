# Neutral review queue actions

## Summary

Reviewers now manage dataset membership with **Add to dataset** and **Remove** instead of recording a user-facing good, bad, or unknown verdict.

## Design

The client maps add to the legacy `good` judgment and remove to `unknown`, preserving the current server contract, queue behavior, undo, prediction evidence, and compensation logic. Verdict-specific action controls and keyboard shortcuts are removed while arrow-key navigation remains available.

The legacy verdict remains a temporary storage detail in the backend and Langfuse. Upcoming custom-criteria work will remove that compatibility layer.

## Migration

No action is required.
