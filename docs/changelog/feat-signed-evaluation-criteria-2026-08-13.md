# Signed evaluation criteria

## Summary

Reviewers can explicitly record either the positive or negative value for each evaluation criterion in the review queue and dataset editor.

## Design

Each criterion offers mutually exclusive positive (`1`) and negative (`-1`) choices. Unselected criteria are omitted rather than stored as zero or inferred as negative. Dataset labels and distributions consume the recorded polarity consistently, while accepted prediction criteria can prefill positive selections for review.

The existing criteria endpoint and storage schema remain unchanged. Shared selection and serialization helpers keep review-queue and dataset editing behavior aligned.

## Migration

No action is required.
