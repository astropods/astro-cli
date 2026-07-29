## Summary

The monitoring page offered Database network summaries and filtering even when an agent had no knowledge base configured. Database monitoring now appears only for deployments whose stored configuration includes knowledge.

## Design

Database monitoring availability is derived from the deployment record, which is the apply-time source of truth. Managed knowledge workloads are identified through the shared workload classifier. Bound knowledge stores, which do not create their own workload, are identified through the server-authored `knowledge_cred` environment provenance on the agent workload.

Inbound and Outbound summaries and filters remain available for every deployment. The Database summary and filter remain visible when knowledge is configured even if the selected window has no traffic, preserving the meaningful empty state for connectivity problems. If a redeploy removes knowledge while Database is selected, the monitor returns to Inbound.

## Migration

No action required.
