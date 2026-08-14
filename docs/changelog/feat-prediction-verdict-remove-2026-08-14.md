# Judged prediction status

## Summary

The eval review queue now separates prediction availability from dataset membership decisions. Reviewers see whether the AI judge analyzed a trace without an overall Good, Bad, or Not sure classification.

## Design

The review queue presents predictions as Judged or Not judged. Judge confidence, explanations, and criterion assessments remain available for each judged trace.

Accepted judge requests mark their traces as Judging immediately. Prediction status polling reconciles each trace with the stored result.

The prediction filter accepts `present` or `absent`. Prediction-present pagination starts from stored prediction records without dataset judgments, then fetches the matching traces. This query fills each page from current review-queue candidates instead of spending its limit on traces with judgments. Prediction-absent pagination scans recent traces and excludes those with stored predictions.

Prediction records retain the overall score for judge compatibility. The review queue does not use that score for presentation or filtering.

## Migration

API clients that filter the review queue by `good`, `bad`, or `unknown` must use `present` or `absent`. No data migration is required.
