# LLM judge onboarding tooltip

## Summary

Introduce auto-judging when a user first opens the LLM judge review queue.

## Design

Run AI Judge shows a reusable coachmark once per authenticated user. Dismissal uses shared persistent storage, and hover guidance remains disabled until onboarding is complete.

Chat and LLM Judge share the coachmark surface and motion. Hover guidance reuses its static styling, built with semantic design tokens.

## Migration

No migration is required.
