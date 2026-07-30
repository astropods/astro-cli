# Consistent column order across Insights tables

## Summary

The Insights page shows three tables (agents, people, models). The agents and models tables order their metric columns Requests, Spend, % Total, but the people table led with Spend and pushed Requests further right, so the same columns landed in different positions when switching views.

## Design

The people table's columns are reordered to the canonical order the agents and models tables already use: identity, then Requests, Spend, % Total, then the table-specific columns (Tokens, Last Used). Header cells and body cells were reordered together so they stay in sync. No data, sorting, or formatting logic changed.

## Migration

None.
