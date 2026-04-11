# Denial State Polish — Blueprint Creation Flow

## Summary

When creating a new blueprint, users could enter a name that already exists, click "Create blueprint," and only receive a denial error on the publishing screen. This surfaces the conflict earlier — inline on step 1 — so users never advance to the publishing screen with an unavailable name.

## Design

A proactive availability check (`useBlueprint`) fires on the setup step as the user types, enabled only when the slug is valid (≥4 chars, starts with a letter). It uses `retry: false` so 404s (name available) resolve immediately without retries.

When the check returns a result (200), `nameIsTaken` is true: an inline error appears below the name field (`acme/my-agent already exists`) and the "Create blueprint" button is disabled. Changing the name or org clears the state automatically via query key change.

The two `useBlueprint` calls (name check and review polling) share the same cache key but have mutually exclusive `enabled` conditions (`setup` vs `review`). A guard on the review navigation effect (`activeStep === "review"`) prevents stale cache from the name check triggering a redirect during setup.

The old error state in the publishing panel has been removed — conflict errors are now fully handled before that step is reached.

## Migration

No action required.
