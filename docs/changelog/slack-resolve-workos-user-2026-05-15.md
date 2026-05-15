# Summary

`/api/v1/deployments/authorize` answered only `{allowed: bool}`, throwing away the resolved WorkOS user_id even though the server had to compute it to evaluate slack identity grants. The messaging container thus had no way to forward the canonical identity downstream for slack traffic — agents and observability saw raw `U…` ids only.

This change extends the endpoint's response to surface the resolved WorkOS user_id on allowed responses so the messaging container can rewrite slack messages to carry the same identity shape as web messages.

# Design

- **Response shape.** Successful `200` responses are now `{allowed: bool, user_id?: string}`. `user_id` is set only on `allowed=true` and only when one is resolvable:
  - `identity_type=user` → echoes back the input (it already *is* the WorkOS user_id).
  - `identity_type=slack` → the workos user linked via `slack_identity_mappings` for `(team_id, slack_user_id)`; omitted when no mapping exists.
  - Anyone-bypass and empty-identity → omitted; no principal was resolved.
- **Deny path stays bare.** Denials return `{allowed: false}` only — no value in surfacing a resolved user_id for an identity that won't get access, and keeping it off the deny path avoids leaking mapping state to a denied caller.
- **No new query params.** Same inputs (`identity_type`, `identity_id`, `identity_scope`, `adapter`); only the response widens. Older messaging containers that ignore `user_id` keep working unchanged.

# Migration

Backward compatible. Existing callers that decode only `allowed` continue to function. The companion change in `modules/messaging` updates the messaging container to read the new field and rewrite `msg.User.Id` accordingly for slack traffic.
