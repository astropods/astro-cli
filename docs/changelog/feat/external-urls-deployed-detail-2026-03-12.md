# External URLs on Deployed Agent Detail

## Summary

Migrates the copyable external endpoint URLs from the legacy Operator UI into the modern Deployed Agent Detail page, moving closer to full Operator deprecation.

## Design

New `ExternalUrls` component in `components/deployed-agent/` renders each `ServiceEndpointInfo` from the deployment's `external_urls` as a thin card row with a link icon, endpoint name, clickable URL, and a copy-to-clipboard button with brief "Copied" feedback. Only renders when the deployment has external URLs. Styling uses theme tokens (`border-border`, `text-muted-foreground`, etc.) consistent with the existing `PodGrid` card pattern.

## Migration

No action required.
