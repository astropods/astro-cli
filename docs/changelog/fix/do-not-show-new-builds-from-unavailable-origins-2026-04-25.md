## Summary

Deployment upgrade signals now preserve the blueprint account that originally produced the deployed build. This prevents same-named blueprints in different accounts from being treated as the same lineage and showing unavailable build updates.

## Design

Deployments carry a `source_account` value in API responses, resolved from `deployments.source_account_id` with a legacy fallback to `deployment_spec_json.source.account`. The client uses that value when loading blueprint versions for dashboard badges and deployment-detail redeploy prompts, so each deployment compares against the blueprint lineage it was actually built from.

Cross-account deployment authorization is also tightened: private blueprints can only be deployed within their own account. Deploying across accounts now requires a public blueprint, regardless of whether the caller belongs to both accounts.

## Migration

No user action is required. Existing deployments continue to resolve their source account from stored deployment specs when the new source-account column is not populated.
