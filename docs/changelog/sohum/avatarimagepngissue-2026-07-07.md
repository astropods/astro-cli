## Summary

Agent badge PNG downloads now preserve the avatar image when the card references app-served avatar assets. The issue showed up when the browser rasterized the generated SVG from a blob URL: relative avatar paths no longer had the page origin as their base and disappeared from the exported PNG.

## Design

The trading-card browser download helper now treats SVG image references as embeddable hrefs instead of only embedding absolute HTTPS URLs. Before rasterizing, it resolves relative URLs against the current page, fetches the image, and replaces the original href with a data URI. Existing failure behavior is preserved: if an image cannot be fetched, the original href remains in the SVG.

The regression test exercises the dashboard share-badge flow end to end. It serves a controlled avatar image, downloads the PNG, and samples the avatar center pixel from the exported file to prove the rasterized output includes the avatar.

## Migration

No user action is required.
