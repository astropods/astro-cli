# Authorization resource backfill

## Summary

Existing organization Accounts and active product resources can now be projected into the Account-rooted WorkOS authorization tree before private-by-default enforcement begins.

## Design

A Queen action named **Connect WorkOS** starts a River job with a 30-minute bound. The job fills missing immutable Blueprint IDs in SQL batches, reads active Astro resources in Account batches, and lists WorkOS once per Account organization. It creates only missing Account, Insights, Blueprint, Deployment, Variable, and Knowledge Store resources, validates existing child parents, and grants `account-admin` to the Account owner through the mirrored WorkOS membership.

Astro remains canonical and no per-resource sync ledger is added. Queen compares the same active Astro resource set with its WorkOS inventory on every read, displaying `missing_in_workos` for an Astro resource that direct registration or backfill missed and `workos_only` for the inverse. Queen stores only the job status and final counts in its existing authorization-operation table. Preview mode performs no writes; failures are collected per resource and make the job fail after the full scan, so a corrected rerun resumes safely. Audience waits for its product table and create API.

## Migration

In Queen, open **Resources**, choose **Connect WorkOS**, preview the connection, then run it. The connection is complete when the operation succeeds and the inventory has no `missing_in_workos` rows. No authorization enforcement changes in this PR.
