# Startup ingestion Jobs: wait for deletion before recreate

## Summary

Redeploys of agents with a `startup` ingestion could fail with `object is being deleted: jobs.batch "<name>" already exists`, reported as a partial failure. Because a startup ingestion maps to a Kubernetes Job with a stable, build-independent name, every redeploy collided with the previous (completed) Job that still lingers under its TTL. The applier deleted the old Job and immediately recreated it, but deletion is asynchronous, so the recreate raced the still-terminating object and failed.

## Design

The fix is contained to the Job apply path. Instead of delete-then-immediately-recreate, the applier now deletes the existing Job and waits until the API confirms it is gone before recreating:

- Deletion uses background propagation so the Job name is freed promptly (old pods are garbage-collected asynchronously; the fresh Job manages its own pods).
- A bounded poll (`Get` until `NotFound`, capped at 60s) gates the recreate, so `Create` is only issued once the object no longer exists. This eliminates the race deterministically rather than shrinking its window.
- The wait is bounded so a stuck finalizer cannot hang the deploy worker.

Job naming, per-build stale cleanup, orphan cleanup, and the existing behavior of re-running a startup ingestion on each redeploy are all unchanged. The manual ingestion trigger path was already race-free (it creates uniquely timestamped Job names) and is untouched.

## Migration

None. Existing deployments benefit automatically on the next redeploy.
