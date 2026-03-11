# Deployment Delete Flow & Shared Confirmation Dialog

## Summary

Adds the ability to delete deployments from the web UI and extracts a reusable confirmation dialog pattern shared across three existing dialogs.

## Design

Each `DeployedAgentCard` now has a dropdown menu with a "Delete" action that opens a destructive confirmation dialog. The dialog requires typing the deployment name and checking an acknowledgement checkbox before the delete button activates. On confirm it calls the `useUndeployAgent` mutation and closes.

The confirmation dialog pattern (type-to-confirm input, checkbox, error display, cancel/action footer) was duplicated across `DeleteAccountDialog`, `ChangeUsernameDialog`, and the new `DeleteDeploymentDialog`. A shared `ConfirmationDialog` component now owns the dialog chrome, checkbox state, and reset-on-close logic. Each consumer passes its own title, description, form fields (via `children`), validation (`canConfirm`), and mutation callbacks.

Server-side, `ListDeployments` now skips Kubernetes namespaces with a non-nil `DeletionTimestamp` to avoid returning stale entries during teardown. `AgentDeployment.id` is now required (non-optional) on both the Go struct and TypeScript type, and is used as the React list key.

The Label `md` variant was updated to a sans-serif form style and adopted in `VariableFields` to replace inline styling.

## Migration

No migration required. The `ConfirmationDialog` is a new component; existing dialog consumers were refactored to use it with no behavioral changes.
