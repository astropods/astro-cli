# Feedback form backend

## Summary

The feedback modal shipped in earlier commits was UI-only — submissions were discarded on the client. This change adds a persistent backend so every submission is stored and queryable.

## Design

A new `feedback_submissions` table stores each submission with the submitting user's ID, email, free-text message, and the page URL where the modal was opened. The table is intentionally user-scoped (not account-scoped) because feedback is about the platform, not a specific account.

The `POST /api/v1/feedback` endpoint sits behind `RequireAuth` and enforces:

- **Message length** — must be 1–5000 characters (server trims whitespace; client also sets `maxLength`).
- **URL length** — page_url capped at 2048 characters.
- **Rate limiting** — max 10 submissions per user per rolling hour, enforced via a count query against an `(user_id, created_at)` index. Returns `429` when exceeded.

On the frontend, `FeedbackModal` now calls a `useSubmitFeedback` mutation hook. The modal shows a loading state while the request is in flight, displays the server error message on failure, and resets all transient state (including the mutation) when closed.

## Migration

Atlas will diff the new `feedback_submissions` table and index automatically — no manual migration steps required.
