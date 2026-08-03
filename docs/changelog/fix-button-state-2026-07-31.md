# Preserve AI judge availability across verdict filters

## Summary

The AI Judge button now reflects whether the complete review queue has any unjudged traces, regardless of the verdict filter currently selected.

## Design

- Disabled the AI Judge button when every item returned by **All verdicts** already has an AI verdict.
- Preserved that disabled state when switching between verdict filters.
- Kept the button enabled when at least one unjudged trace remains.
- Reused the existing review queue request instead of fetching the unfiltered queue a second time.

## Migration

No migration is required.
