## Summary

Removes the headline metrics row (Total Tokens, Total Requests, Total Compute) from the top of the Agents page, and repositions the toolbar so search sits on the left and filters on the right.

## Design

The metrics row was rendered by `DashboardStats`, which composed `MetricCard` and `ComputeUsageCard` and drove two extra API calls — an all-time observability summary and an account usage fetch. These were also pre-fetched in the page loader and primed into the query cache, making the loader do unnecessary parallel work. With `DashboardStats` gone, `ComputeUsageCard` had no remaining consumers and was deleted alongside it. The loader now fetches only deployments, and the cache primer is reduced to match.

The toolbar layout change wraps the status and sort controls in a flex group and adds `justify-between` to the outer container, pushing search to the left edge and filters to the right without changing their individual sizing or responsive behaviour.

## Migration

No action required.
