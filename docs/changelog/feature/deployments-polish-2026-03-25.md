# Deployments Page Polish

## Summary

Visual polish pass on the deployed agent detail page — specifically the Deployments tab and its sub-components.

## Design

**Typography and color:** Command bar label and status text updated to foreground color. Variable names and values reduced to 12px (`--text-mono-sm`). Deployment status stat card changed from `RUNNING` to `Running` (sentence case). Active tab weight reduced from semibold (600) to medium (500) across both the Monitor/Deployments tab bar and the Logs/Variables/Domains inner tabs.

**Layout:** Tab panel backgrounds (Logs, Variables, Domains) unified to `stone-50`. Accordion row hover uses `stone-200` instead of white. History section border only renders when there is content, eliminating a double border at the bottom of the deployment table.

**Components:** Command pill uses `stone-200` fill with muted text. Container dropdown matches the time range select style. Error/warning filter chips simplified to a transparent/neutral style with colored text only. Domains panel URL uses body font (Geist) at body size with foreground color. Services section label uses all-caps label token to match column headers.

## Migration

No migration required.
