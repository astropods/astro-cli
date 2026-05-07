## Summary

Deploy and org secrets settings load account variables in the background. When that request fails (permissions, wrong org scope, network), the UI previously behaved like an empty vault or showed minimal feedback.

## Design

- **Deploy form** (`DeployFormFields`): `useDeployForm` surfaces `vaultEntriesLoadError` from the variables query; an `ErrorPanel` explains load failures above the form; variable rows and `VaultPicker` receive `vaultLoadError` so inline pickers show the same message.
- **Secrets settings** (`VaultSettings`): failed `useAccountVariables` renders `ErrorPanel` with `ApiRequestError.message` instead of an empty state.

Follow-up (separate change): wire vault REST routes to WorkOS **`variable:read`** / **`variable:write`** (initially granted only to org **owner** and **admin** roles), instead of overloading **`org:admin`**.

## Migration

None.
