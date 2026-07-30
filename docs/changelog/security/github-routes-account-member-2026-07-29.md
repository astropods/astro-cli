# Enforce account membership on per-agent GitHub routes

## Summary

The `/agents/:account/:name/github/*` route group resolved the account from the
URL but never verified that the caller belonged to it. Any authenticated user
could therefore act on another tenant's agent: read build logs (which expose
source and secrets) or repoint the agent at a repository they control, so
attacker code would build and run in the victim's namespace. That is a
cross-tenant supply-chain and remote-code-execution path, not just an
information leak.

## Design

Authorization on this surface is layered, and only the outer layer was missing.
`ResolveAccount` turns the URL account slug into an account record; it says
nothing about who is asking. `RequireAccountMember` is the middleware that
answers that question, and every other per-agent route group already applies
it — the GitHub group was the outlier.

The fix applies `RequireAccountMember` at the group level rather than inside
each of the five handlers. Group-level placement is deliberate: it makes the
control structural, so a GitHub route added later inherits the check instead of
depending on the author remembering it. The handlers' account-scoped SQL
(`WHERE account_id = $1 AND agent_name = $2`) remains the second layer,
containing IDOR within an account even if the membership gate were bypassed.

## Migration

No user action required. Callers operating on their own agents are unaffected;
cross-account calls that previously succeeded now correctly fail authorization.
