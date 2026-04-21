## Summary

When deploying an agent, environment variable fields now automatically detect matching vault entries and pre-fill the field with the best match, reducing manual copy-paste of tokens and credentials.

## Design

**Auto-fill on mount**: Each text/secret field runs `useVaultAutoFill` on first render. If the field is empty and vault entries exist, it picks the top suggestion and writes a `{{secrets.NAME}}` / `{{vars.NAME}}` token into the field. The fill is one-shot (a `didAutoFill` ref prevents re-filling after the user clears or replaces the value). `isAutoFilled` is derived as `value === autoFilledToken` rather than tracked with a second effect, which eliminates a race condition where two effects firing in the same render cycle would clobber the label.

**Match tiers** in `findVaultSuggestions`:
1. Exact case-sensitive match (`SLACK_TOKEN` → `SLACK_TOKEN`)
2. Exact case-insensitive match (`slack_token` for field `SLACK_TOKEN`)

Fuzzy/substring matching was evaluated (Jaccard token similarity, trigram similarity) but rejected — it produced too many false positives for variables that share common suffixes like `_KEY` or common prefixes like `OAUTH_CLIENT_`. Exact matching is conservative by design; the picker's search handles the rest.

**Auto-fill indicator**: When a field was auto-filled, a WandSparkles icon and label appear inline inside the `VaultRefChip`:
- Single match → `Auto-filled`
- Multiple matches → `Auto-filled · N other match(es)` as a clickable button that opens the picker.
- The label clears when the user rejects (×) the chip or explicitly selects a different entry from the picker.

**Multi-field stale closure fix**: `VariableFields` uses `useRef` + `useCallback` to track the latest `values` map. Previously, when multiple fields auto-filled in the same effect batch, each write used a stale snapshot of `values` and overwrote the others.

**Picker UX**:
- Selected entry is indicated by a teal left-border bar on the row.
- CaseSensitive icon (`aria-label="Exact match"`, teal) with tooltip "Exact match" for case-sensitive matches.
- CaseLower icon (`aria-label="Close match"`, muted) with tooltip "Case insensitive match" for case-insensitive matches.
- Both indicators are right-aligned inside the entry row.

**Controlled open state**: `VaultPicker` accepts optional `open` / `onOpenChange` props so parent fields can open the picker programmatically (used by the auto-fill label click).

**Deploy payload**: Auto-filled fields are submitted as `{ ref: "NAME" }` (a vault reference), not as a plain value string.

## Migration

No action required.
