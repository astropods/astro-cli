# Eval judge prediction worker

## Summary

Eval dataset predictions can run as independently retryable background work without blocking API requests.

## Design

Each trace is handled by one River job on a dedicated queue. The worker records durable lifecycle state, resolves current trace and feedback context from Langfuse, invokes the account-scoped judge, and stores successful output. Completed predictions remain authoritative across retries, while permanent or exhausted failures become visible through the request state.

```mermaid
flowchart LR
    A["Prediction job starts"] --> B{"Work still needed?"}
    B -- No --> C["Finish without generating"]
    B -- Yes --> D["Load trace and feedback context"]
    D --> E["Get account judge key"]
    E --> F["Generate prediction"]
    F --> G["Store prediction"]
    G --> H["Mark request completed"]
    D -. Failure .-> I["Retry or mark failed"]
    E -. Failure .-> I
    F -. Failure .-> I
    G -. Failure .-> I
```

## Migration

No manual action is required.
