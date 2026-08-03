# Allow clearing an optional variable on redeploy

## Summary

A custom form variable that defaults to empty could not be set back to empty once a previous deployment had given it a value. Clearing the field and redeploying kept the old value.

## Design

When the deploy form built its variables payload, it skipped any field whose value was empty. On a redeploy the server then restored that field from the existing deployment, so a cleared optional field silently kept its old value. The form now sends an explicit empty value for a cleared optional, non-secret field, which mirrors how the knowledge-bindings payload already sends an empty map to preserve explicit clears. A blank secret still means "keep the existing one" and is left out, and required fields are gated by validation, so only user-cleared optionals are affected.

## Migration

None.
