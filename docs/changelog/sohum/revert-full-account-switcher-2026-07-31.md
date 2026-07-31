# Restore active-account scope switching

## Summary

Restore the account-scoped loading model used before the cross-account list filters were introduced. The Agents page could stall even for an account with roughly 50–60 agents because the initial client load expanded across every membership and walked each account's paginated results before settling.

## Design

Account selection is once again an application-level scope change. Switching accounts updates the authenticated account scope and revalidates the current route, so list pages receive server-rendered data for one active account instead of coordinating a client-side multi-account fan-out.

Agents, Blueprints, Knowledge Stores, and Insights now issue bounded requests against the active account. Cross-account aggregation helpers and page-local account filters are removed. Features added after the original account-filter change—including deployment alerts and refresh behavior, persisted non-account Insights filters, Data Sources, Supabase knowledge stores, and editable knowledge credentials—remain intact and use the restored active scope.

## Migration

No user action is required. Account switching returns to the global scope-switching behavior, and list pages load the selected account by default.
