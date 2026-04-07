# Variables & Secrets UI Polish

## Summary

Polish pass on the Variables & Secrets settings page and the VaultPicker component. The primary goal was to tighten language around the "variable vs secret" distinction — leaning into "variable" as the default term while making secret a property you opt into, not a separate concept.

## Design

**Terminology:** Renamed "Secrets & Variables" → "Variables & Secrets" across the nav, page heading, and org settings. The create modal is now "New variable" with a "Secret" toggle rather than framing secret and variable as two separate things. CTAs are unified to "Save" throughout, and the kebab menu actions are simplified to "Edit".

**New variable modal:** Title, placeholder, and CTA updated to remove the secret/variable branching. The Secret toggle tooltip copy is rewritten to be concise and direct ("Hides the value permanently after saving. Can't be read or recovered.") with the inline helper text removed since the tooltip covers it. 1Password autofill is suppressed on value inputs via `data-1p-ignore`.

**Change value modal (secrets):** Added a description field so users can update the description alongside the value. Title format updated to "Change value for NAME". Fixed "cannot" → "can't" in the warning copy.

**Edit variable modal:** Aligned to the same style as the change value modal — matching width, label sizing, and 1Password suppression.

**VaultPicker:** "From [account] vault" simplified to "From [account]" at 11px muted. Description text bumped to 11px. Search input updated to "Find..." with a magnifying glass icon. The VaultRefChip X button now animates smoothly on hover using a width + opacity transition instead of abruptly appearing. Empty state updated to "No variables yet" to match the new terminology, with the "vault settings" link simplified to "settings".

**Empty states:** Both the Variables & Secrets settings page and VaultPicker empty states were updated — icon changed from Lock to KeyRound to match the nav tab, and copy simplified to "No variables yet" / "Create a new variable to get started."

## Migration

No migration required.
