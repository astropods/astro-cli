## Summary

The monitor traces table could compress too far when the detail panel was open, causing adjacent columns and trace ID copy controls to overlap.

## Design

The table keeps its proportional column model but now has a shared minimum width with horizontal scrolling when the available area is too small. Latency and cost receive enough space for their headers, while trace IDs render as a right-aligned tail value such as `...635396f8` with the copy button in normal row flow.

## Migration

No user action is required.
