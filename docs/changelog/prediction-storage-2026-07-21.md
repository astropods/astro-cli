# Eval dataset judge prediction storage

## Summary

Adds the persistence foundation for Astro-managed eval dataset judgment predictions. Predictions can be generated before a reviewer records a verdict and reused by later queue loads, preventing duplicate model spend in subsequent rollout phases.

## Design

Each eval dataset stores at most one prediction per trace, including the numeric verdict score, confidence, explanation, and code-owned judge version. Predicted criterion scores are stored separately using the same server-owned dimensions as reviewer judgment reasons.

Prediction updates preserve the original creation timestamp while replacing the prediction values and complete criterion set atomically. Dataset deletion cascades through predictions and their criteria. Score, confidence, and explanation limits are enforced by database constraints; model-output validation remains the responsibility of the later judge service.

This change only adds schema and storage APIs. It does not invoke a model or alter review queue behavior.

## Migration

No user action is required. The schema migration creates two empty tables and does not modify existing dataset or judgment records.
