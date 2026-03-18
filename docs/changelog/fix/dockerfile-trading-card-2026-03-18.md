# Fix: Add astro-trading-card to astro-client Dockerfile

## Summary

The `astro-client` Dockerfile was missing the `astro-trading-card` workspace package, causing container builds to fail after the trading cards feature was merged.

## Design

Added the package.json copy for dependency resolution and a build step for `astro-trading-card` before the astro-client build, matching the existing pattern for `astro-identity-gen` and `astro-theme`.

## Migration

No migration required.
