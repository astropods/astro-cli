# Knowledge Store Bindings

## Summary

Agents can now bind knowledge entries to managed stores instead of self-hosting them as containers. When bound, the template shapes the entry with just the store ARN (all container fields zeroed), the deploy handler skips container/service creation, and the reference resolver reads DNS and credentials directly from the managed store.

## Design

### Binding flow

The POST template endpoint accepts `bindings.knowledge.<name>` mapping entry names to store ARNs. The server validates (provider match, store status, account ownership), zeros the knowledge entry's container fields, removes its credential variables and editable fields, and returns resolved binding metadata alongside the template. The template's `knowledge` map and the stored deployment spec are identical — bound entries stay with `binding: "arn:..."` and nothing else.

### Reference resolution

`${knowledge.db.host}` resolves to the store's service DNS in the knowledge namespace. `${knowledge.db.http.port}` resolves from the provider registry. `${knowledge.db.credentials.user}` resolves from the store's encrypted credentials, mapped through the provider's `BindCredentials` schema. Credential references route to SecretData automatically.

### Provider credential schema

Each provider declares `BindCredentials` — a list of `{Attr, StorageKey}` pairs mapping reference attributes to the exact credential storage keys:

```
postgres: user→POSTGRES_USER, password→POSTGRES_PASSWORD, database→POSTGRES_DB
qdrant:   api_key→QDRANT__SERVICE__API_KEY
redis:    password→REDIS_PASSWORD
neo4j:    auth→NEO4J_AUTH
```

### Data model

New `knowledge_store_bindings` table (deployment_id, knowledge_name, knowledge_store_id) with CASCADE on deployment delete and RESTRICT on store delete. Stores with active bindings return 409 on delete.

### K8s applier

Bound entries skip service, StatefulSet/Deployment, and credential secret creation. The deployer resolves store info and decrypts credentials before passing them to the applier via `ResolveContext`.

### Client

Binding picker per knowledge entry in the deploy form, filtered by matching provider. Selection triggers an immediate re-POST (structural change). Bindings are seeded from prefill and included in deploy requests.

## Migration

No user action required. Existing deployments are unaffected — bindings are opt-in via the deploy form.
