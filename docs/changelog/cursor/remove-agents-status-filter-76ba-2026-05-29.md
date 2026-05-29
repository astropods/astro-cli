**Summary**

Removes the agents page status filter because the redesigned agent cards no longer surface live status as a primary card concept. The page keeps search and sorting controls while avoiding a stale or misleading filter vocabulary.

**Design**

The dashboard toolbar now renders only the search input and sort select. The agents filter hook keeps text search and sort state, and no longer stores or applies status selections to deployments.

**Migration**

No migration required.
