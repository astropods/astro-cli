## Summary

Vault references (the key icon picker) did not load stored profile variables when clicked inside the Slack credential fields on the deployment page. Clicking the key icon showed "No variables yet" even when the account had secrets configured. All other credential fields on the same page worked correctly.

## Design

`DeployFormFields` renders two separate areas that both use `VariableFields`: the adapter inline-card (via `InterfacesPicker`) and the standalone Configuration / Optional credentials sections. The standalone sections received `vaultEntries` and `vaultSettingsUrl` from the form context, but `InterfacesPicker` had no props for those values and never forwarded them to its internal `VariableFields` renders.

The fix adds `vaultEntries` and `vaultSettingsUrl` props to `InterfacesPicker` and passes them through to both the inline-card and below-card `VariableFields` layouts. `DeployFormFields` then passes `form.vaultEntries` / `form.vaultSettingsUrl` to `InterfacesPicker` the same way it already did for the standalone sections.

Integration tests were added to `DeployBlueprint.test.tsx` covering vault picker behaviour for both Slack credential fields and regular credential fields: entries populate from the API, the empty state renders when there are none, and selecting an entry replaces the input with a vault reference chip.

## Migration

No action required.
