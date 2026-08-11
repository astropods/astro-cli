## Summary

After #1926 shipped, a cluster's `pull_key_hash`/`pull_credential` could be correctly backfilled in the DB, yet deploys targeting it kept failing with "cluster has no registry pull credential." The DB was right; the running process's in-memory registry cache wasn't.

## Design

`k8s.Registry.GetEntry` caches a cluster's row lazily on first read and never expires it until something calls `Refresh`. The boot-time backfill goroutine (`backfillClusterPullCredentials`, `main.go`) wrote the DB but never refreshed the registry — so if a deploy request raced ahead of the backfill and cached the pre-backfill (empty-credential) entry first, it stayed stuck that way indefinitely, surviving even though the DB was subsequently corrected.

Fix: `backfillClusterPullCredentials` now takes the `*k8s.Registry` and calls `Refresh` for every cluster it actually backfills, evicting whatever's cached so the next read reflects the DB. This also required reordering both boot sequences (`runAPI`/`runWorker` in `main.go`) — the backfill goroutine now starts *after* the registry is constructed, instead of racing it.

## Migration

None required. Purely closes a startup race; no schema or API change.
