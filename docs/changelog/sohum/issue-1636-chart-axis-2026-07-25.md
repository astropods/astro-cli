# Preserve request chart axes

Ticket: [#1636](https://github.com/astropods/astro/issues/1636)

## Summary

The Requests & Latency graph now keeps its x-axis visible at desktop and responsive widths instead of collapsing below the space required by the chart.

## Design

The request-volume card owns a minimum chart height, and its grid cell preserves that constraint across container breakpoints. The request chart maps unchanged daily metrics to day indexes, while the token chart retains categorical bands so bar widths and spacing remain stable. Both charts select seven ticks snapped to real days across each time range and reserve edge space so the first and final marks remain inside their chart boundaries.

## Migration

No migration is required.
