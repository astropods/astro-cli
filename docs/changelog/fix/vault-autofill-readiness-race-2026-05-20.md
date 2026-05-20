# Fix vault auto-fill readiness race on deploy page

## Summary

On the deploy page, when a spec variable's name matches a vault secret in the
target account, the field is supposed to auto-fill with a `{{secrets.NAME}}`
reference. Sometimes this didn't happen on first load — the user had to refresh
the page to get the fill. The root cause is a data race between two effects
writing to the same form-state map in the same React commit.

## Design

The auto-fill lives in `useVaultAutoFill` (apps/astro-client/src/components/deploy/VariableField.tsx).
It has a one-shot `didAutoFill = useRef(false)` guard: once the fill writes, it
never tries again.

Two readiness signals feed the deploy form independently:

1. **Template** — fetched via `usePostDeploymentTemplate` (and pre-fetched into
   `initialTemplateResponse` by the React Router loader). When it arrives, the
   parent form's seeding effect runs `applyValues`, which calls
   `setVariableValues(extracted)` / `setAdapterCredentials(extracted)` —
   **full-replacement** (non-updater) setState.
2. **Vault entries** — fetched via `useAccountVariables(targetAccount)`. The
   query is gated on `targetAccount`, which is derived reactively from
   `personalAccount`.

When both signals resolve in the same commit (vault data already in by the
time the template-driven fields mount), React fires child effects before parent
effects:

- **Child** (`useVaultAutoFill`) runs first → condition met → sets
  `didAutoFill.current = true`, calls `onChange(token)`. This queues a
  single-key `setVariableValues({...prev, [key]: token})`.
- **Parent** (template seeding) runs second → queues
  `setVariableValues(extracted)`, where `extracted[key] = ""`.
- React batches both. Non-updater form means the parent's full replacement
  wins. The auto-filled token is gone.
- On the next render, value is `""` again — but `didAutoFill.current` is
  latched to `true`, so the effect short-circuits. The fill never recovers.

The user-visible asymmetry ("refresh works"): on paths where vault data
arrives *after* the seeding effect has already run on an earlier render, the
child fires on a separate commit with no competing setter, so the fill sticks.

### Fix

Gate the auto-fill effect on an explicit readiness flag derived from TanStack
Query's `isSuccess`:

```ts
// useDeployForm.ts
const { data: accountVarsData, isSuccess: accountVarsLoaded } =
  useAccountVariables(targetAccount);
// ...
return {
  vaultEntries: accountVarsData?.variables ?? [],
  vaultEntriesLoaded: accountVarsLoaded,
  // ...
};
```

```ts
// VariableField.tsx — useVaultAutoFill
useEffect(() => {
  if (!entriesLoaded) return;
  if (!didAutoFill.current && value === "" && suggestions.all.length > 0) {
    // ...fill
  }
}, [suggestions, value, onChange, entriesLoaded]);
```

`vaultEntriesLoaded` is threaded through `DeployFormFields` →
(`VariableFields` | `InterfacesPicker` → `VariableFields`) → `VariableField`
→ (`SecretField` | `DefaultTextField`) → `useVaultAutoFill`.

Distinguishing "still loading" from "loaded and empty" eliminates the
same-commit collision: the auto-fill effect cannot fire until the vault query
has resolved, which happens on a render after the parent's seeding effect has
already completed. The child then writes alone, with no competing replacer.

## Migration

None. Behavior change only — auto-fill now fires reliably on first load.
