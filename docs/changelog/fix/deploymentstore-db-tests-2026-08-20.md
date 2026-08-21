# Run the database-backed store tests

## Summary

Four packages under `apps/astro-server/internal` test SQL against a real database. Their `testDB` helper skips when `DATABASE_URL` is unset, so the CI job that runs `./...` ran them as no-ops, and `--hide-summary skipped` kept the skip count out of the output. The job that does provision Postgres, apply the schema, and apply the River migrations only ever pointed at `./e2e/...`.

So the tests had not run in a long time, and twenty-two of them in `deploymentstore` had gone stale against behaviour that changed under them.

## Design

**The integration job covers `./internal/...`.** It already has the database and both migrations applied, so the change is the package list. The unit job keeps running the same packages without a database, where they skip as before.

**What had drifted.** Every failure was a test describing a system that no longer exists, except one:

- Agents became StatefulSets with a default shared disk, so seven tests still expected a Deployment and one fewer volume.
- Superseding a live deployment on insert was replaced by a rejection, with `UpdateDeploymentPending` owning the cleanup. Two tests asserting the old path are gone, and two that meant to test the cleanup now go through the redeploy path that performs it.
- `deployment_variables` was replaced by `deployment_build_env`. The test that existed only to compare the two tables during the dual-write migration is gone; the others read the surviving table.
- Two tests compared `jsonb` byte-for-byte, which Postgres reformats on the way out.
- Seven tests stored secret variables with a nil encryptor. `Encrypt` refuses rather than falling back to plaintext, so they now supply one. The comment on `encryptResolution` still promised that fallback and is corrected, since it is what invited the nil in the first place.

**One real defect.** Reconstructing a variable's targets from its stored role turned `ingestion.<name>` into bare `ingestion`, so a variable scoped to one ingestion component read back as targeting every declared one. Only the admin console reads that field, so the effect was display-only. It now keeps the component name.

## Migration

Nothing required. No production behaviour changes except the reconstructed target list, which the admin console displays.
