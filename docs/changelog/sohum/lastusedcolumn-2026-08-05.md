# Last-used Insights columns

## Summary

The Insights Agents and Models tables now show when each item was last used instead of p95 latency. Full relative times make recent and historical activity easier to scan.

## Design

Agent rows prefer the frequently refreshed observability summary for precise timestamps and fall back to daily Insights history. Model rows derive their latest activity from the selected summary window. Both columns use the same minute, hour, day, month, and year presentation, while existing latency fields remain available in the API for compatibility.

## Migration

No migration is required.
