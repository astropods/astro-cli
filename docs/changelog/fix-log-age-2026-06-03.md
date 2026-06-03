# Fix log viewer time range and add scroll-up pagination

## Summary

The logs viewer in the agent detail page showed the wrong logs when switching time range filters. With `direction=forward` and `limit=200`, Loki returned the oldest 200 lines starting from the `since` timestamp — so a 6h window showed the oldest 200 logs from 6 hours ago, while a 1h window showed the oldest 200 from 1 hour ago. Switching to a wider range made the newest log appear *older*, not give more history.

## Design

**Direction fix**: The deploy logs endpoint now explicitly sets `Direction: "backward"` on the Loki query. Loki returns the most recent N lines in the window, and the existing ascending sort in `QueryLogs` keeps the display order oldest-to-newest. The direction is passed from the client (`direction=backward` query param) and applied in the server handler; other Loki callers (admin gRPC, live tail) are unaffected.

**Pagination**: `useDeploymentLogs` is rewritten with `useInfiniteQuery`. The initial page fetches the 500 most recent logs. When the user scrolls to the top of the log viewer, `fetchNextPage` is called with `until=<oldest visible timestamp>` as the cursor, fetching the next 500 older logs within the same time window. `hasNextPage` is false once a page returns fewer than 500 results (the window is exhausted).

The `until` param was added to the server's `getDeploymentLogs` endpoint — it was already parsed from the query string but not wired from the client.

Scroll position is restored after prepend using `virtualizer.scrollToIndex` so the user stays on the same log line rather than jumping to the top.

**Live mode**: Unaffected. Live tail uses a separate WebSocket path (`TailLogs`), and `useDeploymentLogs` is disabled while tailing.

## Migration

No action required.
