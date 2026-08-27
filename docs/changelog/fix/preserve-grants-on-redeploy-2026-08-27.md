# Summary

Redeploying a Slack-enabled agent silently revoked its web access. The agent kept working for the account owner, so the loss was invisible until someone outside the owning account tried to open the chat page.

# Design

A deploy request configures auth per adapter, but the grant writer treated the presence of any `interfaces.auth` block as "replace every grant for this deployment". Two things then combined against it.

The CLI sends `auth.web = {type: oidc}` on every deploy that enables web, because that is how OIDC is selected. That block carries no grants, but its presence was enough to trigger a full replace.

The server also seeds a Slack `anyone` grant at deploy time (`ensureSlackAnyoneGrant`) so a Slack-enabled agent is reachable, and that seed creates the auth block when one is absent.

So a redeploy submitted a spec whose only grants were the seeded Slack one, and the writer deleted everything else. `web: anyone` became no web grants at all, dropping the deployment onto the per-adapter owner fallback.

- **Replacement is now scoped to the adapters the request actually specifies.** `ReplaceGrantsForAdaptersTx` deletes and rewrites only those adapters' rows. One adapter's block says nothing about another's access, and the same per-adapter scoping already governs the owner fallback.

- **A nil grants list means "leave this adapter alone"; an empty list still means "revoke".** The distinction is what separates the CLI's OIDC-only web block from a deliberate revocation. `adaptersWithGrants` reads it and returns the adapters to rewrite.

- **Fresh deploys are unaffected.** The seed produces a real grants list, so a first deploy still writes its Slack grant, and the UI template path still writes both blocks.

# Migration

None. Existing grants are unchanged, and the first redeploy after this ships stops deleting them.

Deployments that already lost their web grants keep working through the owner fallback but are wider than intended. Re-apply the intended grants from the UI, or with `ast deploy --grant web:anyone` once the CLI flag ships.
