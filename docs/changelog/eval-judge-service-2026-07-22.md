# Eval dataset judge service

## Summary

Adds the server-owned model judgment logic for eval dataset review predictions.

## Design

The service receives a prepared target trace, feedback signals, prior examples, and judge API key, then sends one structured request through Bifrost. It uses a provider-portable schema for response shape while enforcing numeric, cardinality, and explanation-length constraints in server code. It also bounds target input and output, compacts supporting context while preserving both ends of truncated values, and keeps model transport replaceable in tests.

```mermaid
flowchart LR
    A["Receive prepared input and API key"] --> B["Build compact prompt"]
    B --> C["Invoke judge model through Bifrost"]
    C --> D["Decode and validate structured output"]
    D --> E["Return storage-ready prediction"]
```

Trace and feedback loading, prior-example selection, judge-key provisioning, and prediction persistence remain orchestration responsibilities outside this service.

## Migration

No user action is required. The service remains unwired until the prediction API is added.
