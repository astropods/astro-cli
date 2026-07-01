# Bump astro-cli to 0.15.5

## Summary

Version bump for the CLI. The `ast create --model gateway` feature (and the
removal of the `--model ollama` shortcut) merged in #1493 without a
corresponding `VERSION` bump; this carries the release version forward so the
next published CLI reflects those changes.

## Design

Patch bump `0.15.4 → 0.15.5` in `apps/astro-cli/VERSION`, the single source of
truth injected into the binary at build time. No code changes.

## Migration

None.
