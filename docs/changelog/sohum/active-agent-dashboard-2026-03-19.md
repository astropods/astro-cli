## Summary

Adds the active agent monitoring dashboard with real-time metrics, trace viewer, and deployment history tab.

## Design

- **ActiveDetailView.tsx**: Two-tab layout — Monitor (KPI cards + Recharts time-series chart + trace list) and Deployments (past builds with status). Fetches live metrics via existing query hooks. Chart uses `ComposedChart` with an `Area` for request volume and a `Line` for error rate.
- **package.json**: Adds `recharts ^3.8.0` for the metrics chart.

## Migration

Run `bun install` after pulling to pick up the recharts dependency.
