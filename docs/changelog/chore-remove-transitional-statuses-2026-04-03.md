# Remove Transitional Statuses from Dashboard Filter

## Summary
The status filter dropdown on the dashboard previously exposed all eight deployment statuses, including transient ones (Pausing, Restarting, Resuming, Undeploying) that users rarely need to filter on. These have been removed from the filter options.

## Design
`STATUS_OPTIONS` in `DashboardToolbar` is now a hardcoded list of four statuses — Active, Deploying, Error, and Inactive — rather than being derived dynamically from `deploymentStatusLabel`. The underlying filter logic is unchanged: selecting no statuses still shows all deployments (including those in transitional states).

## Migration
No action required.
