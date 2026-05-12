# Fresh deploys default to adapter-appropriate grants

## Summary

A fresh deployment used to seed both web and Slack grants with the same fallback (the deploying user for personal accounts, the whole org for org accounts). That made sense for web — the person creating the agent should be able to open it — but it was wrong for Slack, where the natural install model is workspace-wide. An admin enabling Slack on a fresh deploy had to remember to add an `anyone` grant or nobody in the workspace could invoke the agent.

## Design

Defaults are now picked per adapter, not per account type:

- **Web** → grants the deploying user (`{ user_id: <current user> }`). Same as before, just no longer routed through account type.
- **Slack** → grants `{ anyone: true }`. Matches the typical Slack-app install: anyone in the workspace can use it.

`defaultGrantsForAccount(targetAccountName, accounts, userId)` is replaced by `defaultGrantsForAdapter(adapter, userId)` — account type no longer factors in. The seeding effect calls it twice, once per adapter, so each grant list gets the right default independently.

The same defaults also fire on `setSelectedAdapters` when an adapter is *newly* enabled and its current grants are empty (fresh deploys only). This keeps "turn it on, ship it" consistent whether the adapter was pre-selected at form load or toggled on later. Adapters with non-empty grants are left alone — toggling never overwrites user choices.

Configure-page redeploys are untouched: `isFreshDeploy = !opts.deploymentId && !iv` gates both the seeding and the toggle path, so an existing deployment's grants survive any adapter flip.

## Migration

No migration required. No API, schema, or grant-evaluation changes. Existing deployments continue to use whatever grants they were saved with; the change only affects what the deploy form pre-fills before submit.
