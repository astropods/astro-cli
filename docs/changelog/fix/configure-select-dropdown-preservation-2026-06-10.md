# Configure page: preserve select dropdown values

## Summary

On the agent Configure page, variables declared with `display-as: select` lost their stored value when the page was reopened — the dropdown rendered with the placeholder instead of the previously selected option. Text inputs, number fields, and other field types preserved their values correctly.

The server was correct end-to-end: the value lived in `deployment_build_env`, the `POST /deployment-template` response carried both the value and the schema (`display-as`, `options`). The bug was a render-order race between the form's initial mount and the seeding effect that populates `variableValues` from the template.

## Design

`useDeployForm` initializes `variableValues` to `{}` and then sets the real values in a `useEffect` once the template loads. With that pattern, every controlled input mounts once with an empty value and again with the seeded value.

For most input types that empty→real transition is harmless. Radix Select's hidden form-bubble `<select>` (used for accessibility and form submission) syncs on prop change by calling the native value setter and dispatching a synthetic `change` event. The `change` handler reads `event.target.value` back through `useControllableState` and propagates it as `onValueChange`. Across the `"" → "claude-opus-4-6"` transition the native option list isn't yet registered in a way the setter can resolve, so `event.target.value` comes back `""` — and Radix forwards that empty value back into our state, wiping the seed.

The fix gates `AgentConfigure`'s render on `form.initialValues` being set, not just on the template being loaded. With the gate, every field — Radix Select included — mounts once with its correct value:

```ts
if (form.templateLoading || !form.template || !form.initialValues) {
  return <Loader />;
}
```

The gate is safe under fetch failures: `templateErrorMessage` still surfaces in the same branch, so a stuck loader on error is not introduced.

No changes to `useDeployForm`, `VariableField`, `Select`, or the server. The bug surfaced specifically through `AgentConfigure`, which is the only call site that fetches the template asynchronously without an SSR-prefetched response.

## Migration

None. Existing deployments will see their previously selected dropdown values restored on the next visit to Configure.
