# Queen authorization inventory

## Summary

Queen resource inventory failed because WorkOS no longer accepts an unfiltered authorization-resource list request.

## Design

Astro now loads the distinct WorkOS organization IDs linked to active accounts and lists authorization resources once per organization with bounded concurrency. The results continue through the existing assignment enrichment and short-lived inventory cache. The invalid unfiltered adapter method is removed so future callers cannot repeat the failure.

## Migration

No user action is required. Deploy the updated astro-server before retesting Queen against Preview.
