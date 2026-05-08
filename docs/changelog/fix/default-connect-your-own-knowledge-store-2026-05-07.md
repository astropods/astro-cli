## Summary

The Add Knowledge Store wizard offered two modes for supported providers: Astro-managed provisioning and **connect your own**. Product direction is to standardize on bringing an existing database or vector store only.

## Design

- **`ConfigureForm`**: Removed the Mode chooser and all managed-only fields (storage tier, “Make private”). The form is always the **Connection** flow: name, PrivateLink toggle, host/port and provider-specific credentials, optional skip health check. Submit always calls **`connectKnowledgeStore`**.
- **`ProvisioningStage`**: Dropped the `managed` / `external` branch; copy and step list always match the prior external connect path.

## Migration

None. Managed create APIs remain on the server; they are no longer exposed in this UI.
