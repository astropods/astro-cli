## Summary

Deploy-time knowledge binding and the “add store” wizard needed clearer language and layout: inline provisioning vs account-connected stores, how network exposure works for managed vs external stores, and a clearer path when no compatible store exists yet.

## Design

- **Deploy form knowledge bindings** (`KnowledgeBindingPicker`): Labels are **Local** vs **Shared** (replacing built-in vs existing copy). **Shared** is the default when opening the row so connecting an account store is the obvious path. When **Shared** is selected and there are no ready stores for that provider, the UI shows an empty state with an **Add store** link instead of a disabled-only dropdown; bindings that resolve from the template but are missing from the ready list still surface as a selectable option so edits don’t drop silently. Short explanations use native **`title`** on the two toggles so hover help doesn’t wrap segments or alter the segmented control layout (tooltips remain available on icon toggles that already used them via `ToggleGroupItem`).
- **Word toggle indicator** (`ToggleGroup`): The sliding selection pill still matches the existing segmented styling; its position is derived from **`getBoundingClientRect`** relative to the group root so indicator math stays correct if the active control’s DOM offset chain changes.
- **Add store configuration** (`ConfigureForm` on `/knowledge/new`): Managed **public hostname** vs **private-only** access and external **public internet host** vs **PrivateLink** are chosen with the same explicit radio-row pattern as hosting mode—no switches—so “public vs private” reads consistently end-to-end.

## Migration

None.
