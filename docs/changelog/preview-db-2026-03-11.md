# Direct Atlas migrations for preview environment

## Summary

Preview schema migrations previously uploaded `schema.sql` to S3 and triggered a K8s cronjob to apply it. Now that we have direct database access from CI, this indirection is unnecessary. This change also adds schema plan comments on PRs for better review visibility.

## Design

**deploy-preview.yml** — The S3 upload and K8s job trigger are replaced with a dedicated `migrate` job that runs `atlas schema apply` directly against the preview database. This job runs in parallel with the container build so schema failures are caught early without slowing down image builds. The `deploy` job waits for both `migrate` and `build`, with `if: always()` logic so deploys still proceed when `migrate` is skipped (e.g. client-only or registry-only changes).

**sql-review-action.yml** — A new `schema-plan` job runs on PRs that touch `schema.sql`. It diffs the preview database against the proposed schema using `atlas schema diff` and posts a formatted PR comment showing:
- The exact SQL that will be applied on merge
- Change statistics (additions, removals, alterations)
- Error details in a collapsible section if Atlas fails

The comment uses a hidden HTML marker so it updates in place on subsequent pushes.

Secrets required in the `preview` GitHub environment: `DATABASE_URL`, `DEV_DATABASE_URL`.

## Migration

No action required. The S3 bucket and `astro-schema-migrate` cronjob in the preview cluster can be decommissioned once this is verified working.
