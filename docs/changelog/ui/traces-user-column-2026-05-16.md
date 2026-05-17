# Show user behind each trace in the monitor table

## Summary

Langfuse now stores a `user_id` on each trace (the agent passes the caller's WorkOS user ID). The monitor page's trace table previously had no signal for *who* the trace was for, which made multi-user agents hard to triage. This change surfaces that user as a real profile (avatar + display name) rather than a raw opaque ID.

## Design

The Langfuse trace listing already returns `userId`, so the server change is small: `langfuse.Trace.UserID`, `handlers.TraceEntry.UserID` (omitempty), and the inline projection in `GetLangfuseTraces` all carry it through. The TS `TraceEntry` mirrors with an optional `user_id`.

The interesting choice is on the client. The WorkOS user ID is already the value stored in `account_members.user_id`, so the existing members endpoint (`useAccountMembers(account)`) is the natural lookup — no new endpoint, no per-row fetch. A new `<UserBadge userId account />` component:

- Calls `useAccountMembers(account)` once (TanStack dedupes across rows in the table).
- Finds the member with matching `user_id`, then renders `<UserAvatar handle name />` + display name.
- Falls back to a muted, monospace raw `user_id` when no member matches (e.g. former member, cross-org caller). Empty `userId` renders as em-dash; loading renders as `…`.

`TracesTable` takes a new `account` prop; `AgentMonitor` passes it from the existing `useAgentDetailContext()`.

## Migration

None.
