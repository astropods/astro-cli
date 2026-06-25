# E2E regression test: blueprint wizard carousel alignment

## Summary

Adds a Playwright e2e test asserting the blueprint create wizard's step carousel
stays aligned after navigating Back then Continue.

This PR is branched off `main`, which does **not** contain the carousel fix, so
the test is expected to **fail** here — that failure confirms it catches the
regression. The fix is in a separate PR; once it lands the test passes.

## Design

The test drives the reported repro (Continue → Set up with GitHub → Back →
Continue) and asserts the source step is in the viewport and the carousel
viewport's `scrollLeft` is 0 (the direct signal of the misalignment).

## Migration

None — test-only.
