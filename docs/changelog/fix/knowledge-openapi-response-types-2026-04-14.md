# Fix knowledge OpenAPI response types

## Summary

All eight knowledge store endpoints in the OpenAPI spec annotations were using `ErrorResponse` as the success (200/202) response type. This produced incorrect API documentation and would generate wrong client types.

## Design

Exported the previously-unexported `knowledgeResponse` and `knowledgeEvent` types as `KnowledgeResponse` and `KnowledgeEvent`, added `KnowledgeCredentialsResponse` for the credentials endpoint, and corrected each endpoint's `oapispec.Response` call to reference the actual type the handler returns:

- Create/Connect/Get: `KnowledgeResponse`
- List: `[]KnowledgeResponse`
- Delete: `MessageResponse`
- Logs/Events (SSE streams): `nil`
- Credentials: `KnowledgeCredentialsResponse`

Error-status annotations (400, 404, 409) correctly used `ErrorResponse` and were left unchanged.

## Migration

No migration required. This only affects generated API documentation; runtime behavior is unchanged.
