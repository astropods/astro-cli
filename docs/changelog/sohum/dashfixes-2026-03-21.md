# sohum/dashfixes

## Summary

This update stabilizes the active agent dashboard around predictable deployment lifecycle behavior. The goal is to preserve the established status semantics from the previous UI while improving real-time feedback during undeploy and deployment inspection.

## Design

The client now treats deployment status as a shared contract between list cards, detail header, and deployment history surfaces. Status rendering is normalized through common deployment status mapping so headline status indicators and row-level status labels do not drift from one another during polling and transitions.

Undeploy now uses an explicit transitional state in the UI. Instead of disappearing immediately after delete is requested, a deployment remains visible as `Undeploying` until the backend transitions it out of the live set. The deployments query polls while transitional states are present so removal happens without a hard page refresh.

Deployments detail language is aligned to the pod-first presentation model. The page communicates pod-centric operational context while keeping per-container logs and controls inside each pod section, reducing ambiguity in multi-workload deployments.

## Migration

No migration steps are required. This is a behavior and UX consistency update only.
