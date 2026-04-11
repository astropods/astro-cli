# Fix: Persistent knowledge containers missing environment variables

## Summary

Persistent knowledge containers (StatefulSets) were not receiving their `inputs` as environment variables in Kubernetes deployments. This caused containers like Postgres to fail on startup with errors like "superuser password is not specified" — even when `POSTGRES_PASSWORD` was declared as an input with a default value in `astropods.yml`.

The same configuration worked correctly in local development via `ast dev` (Docker Compose).

## What happened

When building the Kubernetes StatefulSet for a persistent knowledge container, the `Environment` field was not being passed to the container config. The environment map — which the template generator had already populated with the knowledge inputs — was silently dropped.

Non-persistent knowledge containers (Deployments) and persistent model containers (StatefulSets) both correctly passed the `Environment` field. Only the persistent knowledge path was missing it.

## Fix

One-line change in `apps/astro-server/internal/k8s/spec_applier.go`: added `Environment: knowledge.Environment` to the container config in the persistent knowledge StatefulSet path, matching the pattern used by every other container type.

## Migration

No migration required. Redeploy any agent with persistent knowledge containers that use `inputs` — they will now receive the expected environment variables.
