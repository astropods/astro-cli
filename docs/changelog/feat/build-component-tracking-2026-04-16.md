# Per-Component Build Tracking with Persistent Logs

## Summary

GitHub builds can have multiple components (agent, tools, models, ingestion), but the system tracked them as a single record with one status string. Logs were ephemeral — fetched from K8s pods and lost after GC. The build logs endpoint also hardcoded the `-agent` suffix, making other component logs invisible.

## Design

New `github_build_components` table stores per-component status, K8s job name, and full logs. The worker creates component records upfront so the UI shows all components immediately, then updates each through `pending → building → succeeded/failed` with logs persisted after each build completes (truncated at 512KB).

`RunJob` now returns `(logs, error)` — logs are captured on both success and failure paths while the pod still exists. The build logs handler serves persisted logs from the DB, falling back to live K8s fetch for in-progress builds. K8s events are placed first in log output for faster debugging of scheduling/pull issues.

Frontend shows per-component status pills (with spinner/check/x icons) in the build row, and the log dialog renders per-component collapsible sections with nested container-level logs. Old builds without components degrade gracefully to the flat log view.

## Migration

Run the following DDL to add the new table:

```sql
CREATE TABLE public.github_build_components (
    id bigserial NOT NULL,
    build_id uuid NOT NULL,
    component_name varchar NOT NULL,
    status varchar NOT NULL DEFAULT 'pending',
    k8s_job_name varchar NOT NULL DEFAULT '',
    logs text NOT NULL DEFAULT '',
    started_at timestamp,
    completed_at timestamp,
    CONSTRAINT github_build_components_pkey PRIMARY KEY (id),
    CONSTRAINT github_build_components_build_fkey FOREIGN KEY (build_id)
        REFERENCES public.github_builds(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_build_components_build_name ON public.github_build_components(build_id, component_name);
```

No data migration needed — existing builds simply have zero component rows and the UI falls back to the flat log format.
