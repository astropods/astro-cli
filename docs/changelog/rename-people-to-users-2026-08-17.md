# Rename the Insights "People" label to "Users"

## Summary

The Insights view labeled "People" listed Slack bots and agent identities
alongside human members, so the term "People" undersold what the tab actually
showed. The user-facing copy is now "Users" to reflect the full set of entities
displayed there.

## Design

The change is label-only — the underlying view key was already `users`
(`ActivityView = "agents" | "users" | "models"`), so no state, routing, or query
params changed. The renamed strings are:

- `ViewToggle` — the view pill reads **Users** instead of **People**.
- `ActiveUsersSpendChart` — the card heading is **Users spend over time**, and the
  legend/tooltip series label is **By Users**.
- `TopSpendersTable` — the empty state reads *No activity from users in this
  period*, and the "used by" overflow popover counts **users** (was *people*).

## Migration

None. Presentation-only.
