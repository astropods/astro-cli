# Smoke test: blueprint setup screen scrolls to its action buttons

## Summary

Adds an auth-gated Playwright smoke test asserting that the "Setup your agent
blueprint" screen can be scrolled so its bottom action buttons are reachable.

This PR is branched off `main`, which does **not** contain the scroll fix, so the
test is expected to **fail** here — that failure confirms the test actually
catches the regression. The fix lives in a separate PR; once it lands the test
will pass.

## Design

The test loads `/new/custom` at a short viewport (so the primary action sits
below the fold), performs a real wheel scroll, and asserts the Continue button
is in the viewport. A wheel event does not move an `overflow-hidden` container,
so the assertion only passes when the screen is genuinely scrollable.

## Migration

None — test-only.
