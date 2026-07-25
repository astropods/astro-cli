# Distinguish human users from bots and agents on Insights

## Summary

The Insights People table listed real people, Slack bots, and agent identities without any visual distinction, so spend and usage metrics blurred human engagement with automated traffic. Non-human identities now carry a small badge, so the table reads at a glance.

## Design

A shared classifier in `lib/identity-kind.ts` decides whether an Insights identity is non-human: `kind` is `agent` or `system`, or the resolved Slack `user_details.is_bot` is true. A companion `nonHumanLabel` returns the badge text — `Agent` for agent identities, `Bot` for Slack bots — and null for humans (and for `system` rows, which already carry their own marker).

The People table's identity cell renders that badge next to the name when present. The badge lives in the row renderer shared by the People table and the "used by" overflow list, so both surfaces mark bots consistently. No new data is fetched: `kind` and `user_details` are already part of the insights payload.

A server-side filter (hide bots) is intentionally out of scope here: the People count is server-paginated, so a client-only filter would desync the count. That belongs with a backend query parameter.

## Migration

None. Presentation-only.
