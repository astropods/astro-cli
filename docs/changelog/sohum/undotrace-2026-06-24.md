## Summary

Dataset review now separates fast mistake recovery from longer-term dataset maintenance. Reviewers get a temporary undo affordance immediately after judging from the review queue, while the dataset tab exposes deliberate row actions for changing a verdict or removing a trace from the dataset.

## Design

The server exposes two deployment-scoped judgment maintenance paths. Undo deletes the local judgment gate atomically while returning the stored verdict, decrements the cached good or bad count when needed, and removes the deterministic Langfuse dataset item for scored judgments. Once the judgment row is gone, the existing review queue filter naturally lets the trace reappear; queue ordering still comes from the Langfuse trace timestamp path already used by the review queue.

Changing a verdict updates the local judgment, cached counts, and deterministic Langfuse dataset item in place. Good and bad keep the trace in the scored dataset; neutral stores an unscored judgment and removes the scored dataset item without returning the trace to the queue. The client refreshes the dataset summary and item pages without invalidating the review queue, so this path behaves like a correction instead of a requeue.

The review queue shows a temporary undo affordance after a verdict is saved. This mirrors the lightweight prototype flow: reviewers can reverse a just-made judgment without switching tabs or hunting through the dataset. The dataset tab uses a compact three-dot menu per row with `Change verdict` and `Remove from dataset`; remove reuses the eval flyaway motion system with a review-queue target, giving reviewers a visual cue that the trace is moving back into the queue instead of toward the grade.

## Migration

No user action is required.
