# Deploy Setup Page & Agent Card Polish

## Summary

Visual polish pass across the deploy setup form, deployed agent cards, and the monitor page.

## Design

- **FormSection**: section headings softened from bold to semi-bold; description text uses `muted-foreground` instead of `faint-foreground` for better readability
- **DeploymentStatusBadge**: new shared component replacing `StatusIndicator` on agent cards — renders a pill-style badge (rounded, colored bg/border, dot or spinner) consistent with the detail page header badge; 11px mono, sentence case
- **Monitor page**: Request volume, Token usage, and Traces card headings set to semi-bold; empty state icon uses a square border radius (`8px`) instead of full circle

## Migration

No API or data model changes. No migration required.
