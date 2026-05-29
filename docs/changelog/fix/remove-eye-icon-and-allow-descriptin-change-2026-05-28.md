# Vault secret edit: remove reveal toggle, allow description-only updates

## Summary

The vault edit dialog for secrets showed a reveal toggle on an empty password field with a placeholder. Users expected it to show the stored value, but server vault secrets are write-only — values are never returned on read. Save also required entering a new value even when the user only wanted to change the description, which the API already supported.

## Design

**Write-only vault model unchanged.** Account secrets remain encrypted at rest and are only decrypted server-side at deploy time. GET `/variables` continues to return metadata only (name, type, description, timestamps). The UI must not imply that stored secret values can be viewed or prefilled.

**Edit dialog UX.** The secret edit dialog is reframed as "Edit {name}" rather than "Change value". Copy explains that leaving the value blank keeps the current secret; entering a value replaces it permanently. The reveal toggle is removed — it could never show the stored value and was misleading.

**Partial updates.** Save is enabled when either the description changed or a new value was entered. On confirm, the client sends `{ description }` only when the value field is blank, or `{ value, secret: true, description }` when the user entered a new value. The server's existing `UpdateAccountVariable` handler already treats `value` as optional and updates only the fields provided.

Plain variables continue to use `EditVariableDialog`, which prefills the value from the API since non-secret variables are readable.

## Migration

None.
