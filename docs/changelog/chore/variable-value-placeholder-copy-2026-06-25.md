## Summary

Small polish pass on the variables/secrets dialogs in account settings. The secret-overwrite value field used a row of bullet dots as its placeholder, which read as a masked value rather than guidance, and the "Add description" toggle in the new-entry dialog had no horizontal padding so its hover fill clipped against the field column.

## Design

- The overwrite-secret value input now uses a plain text placeholder ("Enter a value") instead of the bullet-dot string, so the empty state reads as a prompt rather than a pre-filled secret.
- The "Add description" ghost button picks up `px-3.5`, matching the inputs' internal padding, so its label aligns with the Key/Value placeholder text and the hover fill keeps its rounded corners inside the column.

## Migration

No action required.
