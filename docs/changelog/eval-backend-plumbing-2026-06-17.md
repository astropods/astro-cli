## Summary

Adds the backend foundations for judged eval datasets without changing the public dataset API surface. The change prepares Astro to track human judgments locally before writing judged examples to Langfuse, preventing duplicate trace judgments from drifting local and upstream state.

## Design

The eval dataset data access layer is renamed from `datasetstore` to `evaldatasetstore` so it is clearly scoped to eval datasets. Dataset records now track cached good and bad judgment counts alongside the existing Langfuse dataset name, enforce non-negative cached counts, and each dataset row has a stable UUID identity for downstream relationships.

Judgment deduplication is handled by a new `judgmentstore` package backed by an `eval_dataset_judgments` table keyed by eval dataset and trace. The store provides insert, delete, and lookup helpers so future API handlers can gate duplicate judgments locally before writing to Langfuse.

Dataset naming and legacy repair are centralized in `evaldataset.Ensure`. Deploy-time provisioning now calls this shared ensure path, which creates missing canonical `eval-*` Langfuse datasets and best-effort heals older `dep-*` rows without failing deploys.

The `evaldataset` package also owns dataset grading and user-response sentiment helpers so future queue and summary endpoints can share the same scoring and signal logic.

## Migration

No user action is required. Existing eval dataset rows continue to work; legacy rows are healed opportunistically on deploy.
