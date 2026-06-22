# Summary

Removes category filter selection from the Explore blueprints page while keeping search and sort intact.

# Design

Explore returns to a simpler toolbar: search on the left and sort on the right. Blueprint tags still participate in text search, but they no longer create a selectable filter dropdown or alter the result set independently of search.

The shared multi-select component remains available for other product surfaces; this change only removes its usage from Explore.

# Migration

No user migration required.
