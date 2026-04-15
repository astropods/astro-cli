# Bulk variable creation and import UX overhaul

## Summary

The variable creation dialog was reworked to support creating multiple variables at once, with a new bulk API endpoint to back it. The previous separate "Import .env" dialog was removed in favor of a unified flow inside the "New variable" dialog. Validation was improved to show inline errors on blur and on submit.

## Design

### Bulk API

The `POST /api/v1/accounts/:account/variables` endpoint now accepts `{ variables: [...] }` instead of a single entry. The server iterates each entry, validates, encrypts secrets (reusing a single encryptor instance for the batch), and returns per-entry results: `{ results: [{ name, status, error? }] }`. This replaces the client-side `Promise.allSettled` pattern that fired N individual requests.

### Unified create + import flow

The `NewEntryDialog` now handles both manual entry and import:

- **Import dropdown** in the dialog header with "Upload file" and "Paste text" options
- File upload uses the existing `.env` parser; paste opens a secondary dialog
- Imported rows are appended to existing rows (not replaced)
- 30-row limit enforced across manual and import entry

### Validation

- Fields show inline errors with destructive ring styling on blur (`touched` state per field)
- Imported rows are auto-marked as touched so errors show immediately
- Save button is always enabled; clicking it with errors reveals all inline errors and scrolls to the first one
- Variable names now allow mixed case (`^[a-zA-Z_][a-zA-Z0-9_]*$`) on both client and server

### Removed code

- `ImportEnvDialog.tsx` deleted — it was rewritten on this branch but never imported anywhere
- Unused `Upload` icon import removed from `SecretsSettings.tsx`

## Migration

No user action required. The API change is backwards-incompatible but the only consumers are the web client (updated in this PR) and the mock backend (also updated).
