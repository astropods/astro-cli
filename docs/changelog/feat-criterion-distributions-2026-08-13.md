# Criterion distributions

## Summary

The dataset summary now describes recorded evaluation criteria instead of presenting an aggregate grade.

## Design

Each evaluated criterion is rendered in its defined order with positive and negative counts, a reduced ratio, and a distribution bar. Each criterion uses its own evaluated total as the denominator, so omitted criteria do not affect other distributions. Criteria with no recorded values are hidden, and a neutral empty state appears when none have values.

The existing `criteria_counts` response remains the source of truth; no API or schema changes are required.

## Migration

No action is required.
