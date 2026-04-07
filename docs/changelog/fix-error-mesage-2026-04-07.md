## Summary

Deploy validation errors showed only a generic error code (e.g. "invalid deployment spec") with no indication of what specifically was wrong. The specific diagnostic message from the server was being silently discarded.

## Design

The deploy error state was a flat string. It is now `{ message: string; details?: string }`, separating the top-level error from the diagnostic detail:

- `message` — the error code, formatted via `sentenceCase` (e.g. `"validation_failed"` → `"Validation failed"`)
- `details` — the specific diagnostic (e.g. `"variables.GITHUB_TOKEN.value: required variable has no value"`) or individual `validation_errors` lines

The `ErrorPanel` uses `message` as its title and `details` as the body, so users see the full picture at a glance.

## Migration

No changes required.
