## Summary

The `ast dev` ready banner showed `http://localhost:3000` without a label and described `http://localhost:3100` only as `(API)`, making it unclear which service each port served.

## Design

Updated `printReadyBlock` in `apps/astro-cli/cmd/dev.go` to label both ports consistently with the rest of the banner:

- `http://localhost:3000` → labeled `(playground)`
- `http://localhost:3100` → labeled `(messaging API)`

## Migration

No action required.
