# Simplify eval judge context

## Summary

The eval judge and review queue no longer carry unused sentiment and prior-example data.

## Design

- Removed heuristic sentiment classification while retaining the raw next-user message used by the judge.
- Removed the unused prior-example payload from judge requests.
- Removed sentiment from review queue responses and client models.
- Restored review queue items by timestamp without sentiment-based ordering.

## Migration

Review queue API consumers should stop reading the removed `sentiment` field.
