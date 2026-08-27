# FGA resource inventory and role ladder

## Summary

The private-by-default spec named roles for Account, Blueprint, and Deployment only. Every other thing a customer owns had no place in the model, and the three-role bundle jumped from editing a resource to deleting it and granting access in one step. This fills in the inventory and replaces the bundle with a ladder.

## Design

Four more Account-parented resource types: Variable, Audience, Insights, and Knowledge store. A type earns its own entry when people grant it one at a time. Everything under a Deployment or a Blueprint stays with its parent, so datasets, files, traces, chat history, versions, and builds get no type of their own.

Resource roles become the GitHub ladder: Viewer, Writer, Maintainer, Admin, each rung adding to the one before it. Maintainer holds `operate`, which acts on the running thing without changing it. Admin holds `delete` and `manage_access`. A type skips a rung when it has no action at that level, so Variable has no Maintainer and read-only Insights has neither Writer nor Maintainer.

Account gains `account-maintainer`, and only `account-admin` inherits child permissions. This is the part that closes a hole in private-by-default: administering the team no longer implies read on every deployment's traces and every variable. Admin stays the only role that deletes the account, changes the payment method, and recovers a resource whose admins have all left.

Every slug is also a machine-app scope, because the app-scopes endpoint returns the WorkOS permission list verbatim. That settles the vocabulary the machine credentials and access audiences specs both left open, and it is why slugs read as an API author would expect: `data_source:*` follows the Data Sources page rather than the `otel_ingest_tokens` table, and access groups become plain groups.

A deployment spec reaches outside itself to a variable, a knowledge store, or an audience. None of those can be a child of every deployment that uses it, so whether deploy requires `<type>:read` on each referenced resource stays an Astro policy check rather than a WorkOS relation, and stays open.

[WorkOS FGA setup](../../05-implementation/workos-fga-setup.md) carries the same model as flat slug, name, and description tables for pasting into the WorkOS dashboard. The spec explains the design; the setup page holds only values, so the two do not drift into two explanations.

`scripts/workos-fga/model.json` is the machine-readable copy, and `apply-permissions.sh` creates the permissions through `POST /authorization/permissions`. Resource types have no public API and stay a dashboard step. The script targets staging by default and takes `--prod`, and it compares the environment ID from the credential rather than a name, because every project on the team has an environment called Staging and retargeting a CLI profile does not swap its stored API key.

## Migration

None. Planning documentation only. The new types are marked Later; V1 remains Account, Blueprint, and Deployment.
