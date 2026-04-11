# Fix: Postgres provider fails on Kubernetes due to lost+found

## Summary

The Postgres provider's volume is mounted directly at `/var/lib/postgresql/data`, which is also Postgres' default `PGDATA` directory. On Kubernetes, the persistent volume's ext4 filesystem creates a `lost+found` directory at the mount root. Postgres' `initdb` refuses to initialize into a non-empty directory, causing the container to crash loop.

## Fix

Added `PGDATA=/var/lib/postgresql/data/pgdata` to the Postgres provider's `DefaultEnv`. This is the standard Kubernetes workaround — used by the Bitnami Postgres Helm chart and documented on the official Postgres Docker Hub page. Postgres initializes into a clean subdirectory under the mount point, avoiding the `lost+found` conflict.

## Migration

No migration required. Existing persistent volumes are unaffected — if Postgres previously initialized at the mount root (Docker Compose, where `lost+found` doesn't exist), it will continue to find its data there. New Kubernetes deployments will initialize into the `pgdata` subdirectory.
