# Allow clearing all knowledge bindings on redeploy

## Summary

A deployment that already had knowledge-store bindings could not have all
of its bindings removed. Both the template-shaping endpoint and the
deploy persistence collapsed "no bindings sent" with "user wants no
bindings" and silently restored the stored bindings, making the
redeploy-with-no-bindings path impossible to express end-to-end.

## Design

The wire contract is now: a missing `bindings` field means "client has
no input — restore from the stored deployment spec" (initial-load
behavior); any present `bindings.knowledge`, including `{}`, means
"explicit intent — use exactly this." That single rule is enforced at
three layers:

- **Template handler** — `ApplyStoredBindingsToRequest` only seeds from
  the stored spec when `req.Bindings == nil`. Empty maps and empty-string
  ARNs are explicit clears, no longer overridden.
- **Deploy persistence** — `SaveBindings` runs on every deploy when the
  knowledge-store is configured, with a possibly-empty map. The replace-
  style DELETE+INSERT in `SaveBindings` already handled clearing; the
  handler just had to stop guarding the call on `len > 0`.
- **Frontend (`useDeployForm`)** — adapter changes, binding edits, and
  submit always send `bindings: { knowledge: cleaned }` (possibly empty).
  The initial template fetch is the only path that omits `bindings`, so
  stored-spec restore only happens on first load.

## Migration

None. Clients sending `bindings: undefined` on initial load still get
stored bindings restored as before.
