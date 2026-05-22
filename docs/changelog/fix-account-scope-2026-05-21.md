## Summary

The per-agent infrastructure usage endpoint rejected requests when the blueprint lived in a different account, even if the requesting account had active deployments of that blueprint. This blocked legitimate cross-account deployment usage queries.

## Design

Billing events are emitted with `account_id = deployments.account_id`, so a deployment's compute hours are already attributed to the hosting account regardless of where the blueprint originated. The OpenMeter query was already correctly scoped to the requesting account via `Subject: acct.ID` — the only problem was a pre-query validation that checked for blueprint existence in the requesting account's index, which failed for cross-account blueprints.

The fix removes that validation from `GetInfrastructureUsage`. The `index *agentindex.Index` parameter is dropped from the handler since it was only used for this check. If an `agent_name` has no usage events in the account, OpenMeter returns zero, which is handled gracefully.

## Migration

No action required.
