## Summary

Insights could show bare Slack users without profile deep links when those users had historical Langfuse traces but no row in the Slack observed-user directory. The directory is populated by live Slack authorization, so a user who recently invoked an agent got a deep link while inactive historical users from the same workspace did not.

## Design

The users-summary server path now keeps the existing exact lookup first: linked Slack rows still merge into WorkOS users, and observed rows still provide their own `slack_team_id`. After that exact pass, the server computes a conservative fallback from the directory hits in the scoped Insights result. If every known Slack row points to the same `team_id`, any remaining bare Slack rows inherit that team for deep-link rendering.

The fallback is intentionally narrow. If no team is known, or multiple teams are present, missing rows remain unlinked. Exact directory hits are never overwritten, so once live-ingest writes a user's own `(team_id, slack_user_id)` row, that explicit row wins.

## Migration

No migration required.
