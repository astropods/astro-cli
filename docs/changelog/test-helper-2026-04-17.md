## Summary

Adds a developer utility for reviewing Playwright test traces interactively without needing to remember long file paths.

## Design

`apps/astro-client/scripts/review-test-trace.sh` scans the `test-results/` directory for `trace.zip` files, presents a numbered menu of available traces (named after their test), and opens the selected trace in the Playwright trace viewer. The script loops so multiple traces can be opened in sequence without restarting; each viewer launches in the background so the menu stays live.

Usage:

```sh
# Run tests with tracing enabled
VITE_API_URL='' bun x playwright test --trace on

# Open the interactive picker
./scripts/review-test-trace.sh

# Optionally point at a different results dir
./scripts/review-test-trace.sh path/to/test-results
```

Press `q` to exit. Each selection opens a browser tab via `bun x playwright show-trace`.

## Migration

No action required.
