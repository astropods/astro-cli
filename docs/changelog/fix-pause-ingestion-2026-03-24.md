# Pause also suspends ingestion CronJobs

## Summary

Pausing a deployment only scaled agent workloads (Deployments) to zero but left scheduled ingestion CronJobs running. This meant ingestion pipelines continued executing even while the agent was paused.

## Design

The pause handler (`PauseDeployment`) now lists all CronJobs in the deployment namespace with the `app.kubernetes.io/managed-by=astro-server` label and sets `Spec.Suspend = true` on each before marking the deployment as `scaled_down`. This prevents the scheduler from firing new ingestion runs while paused.

Resume (wakeup) calls `deployer.Apply()` which re-applies the full spec — this updates CronJobs with `Suspend` unset (defaults to false), unsuspending them. A secondary fix was applied to `applyCronJob` in the K8s applier: it now fetches the existing resource and sets `ResourceVersion` before updating, which was previously missing and would have caused the CronJob update to fail silently on wake-up.

## Migration

No migration required. Existing paused deployments will have their CronJobs suspended on the next pause action; resume is handled automatically by the wakeup worker.
