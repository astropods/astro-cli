# Fix: smoke test for the "Book a demo" CTA

## Summary

The production smoke test asserted that the homepage "Book a demo" CTA was a
link pointing at the Google Calendar scheduler (`calendar.app.google`). The
marketing site replaced that external link with a gated demo request form
(`BookDemoButton` → modal `LeadForm`), so the CTA is now a button that opens a
modal, not a link — the smoke test failed on `main` against the live site.

## Design

Update `apps/tests/smoke/public.spec.ts` to match the shipped behavior: assert
the "Book a demo" **button** is visible, click it, and assert the demo request
modal opens (heading "Schedule a personalized demo"). This keeps the smoke test
meaningful (the CTA still reaches the demo flow) without depending on an external
scheduler URL.

## Migration

None. Test-only change.
