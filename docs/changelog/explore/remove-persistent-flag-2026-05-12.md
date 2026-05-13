## Summary

The `persistent` flag in `astropods.yml` (on `knowledge.<name>` and `*.container`) duplicated information the platform already had. For registered providers (qdrant, redis, postgres, mysql, neo4j), persistence is intrinsic — every one of them declares a `MountPath` in the provider registry. For user-supplied containers, the user already names a mount path via `volume`. The flag forced users to declare something the platform can derive, and the two signals could disagree.

This change removes `persistent` from the user-facing spec and derives it from the mount-path signal in both modes.

## Design

**The rule:** a knowledge workload is persistent iff it has a mount path.

- **Provider mode** (`knowledge.foo.provider: qdrant`): persistence is derived from `BuiltinProvider.MountPath != ""`. Every self-hosted knowledge provider in the registry today declares a mount path, so all of them become persistent.
- **Container mode** (`knowledge.foo.container.image: my-thing`): persistence is derived from `container.volume != ""`. Setting a mount path is the user's declaration of intent to persist.

The `storage:` block (PVC size, class, access mode) stays. It is now PVC tuning for an already-persistent workload, not a separate signal. Defaults apply when omitted.

**Spec changes** (`packages/astro-spec`):
- `Knowledge.Persistent` is removed.
- `ContainerConfig.Persistent` is tagged `json:"-" yaml:"-"` — kept as a derived field, not user-settable.
- `Knowledge.ResolvedContainer()` derives `Persistent` from `Volume`; in provider mode it also copies `prov.MountPath` into `Volume` so the invariant `Persistent ⇔ Volume != ""` holds uniformly across both modes. Downstream callers that previously fell back from `container.volume` to `provider.MountPath` (the normalizer's `buildDeploymentKnowledge`, the compose builder) just read `container.Volume` directly.

**Normalized deployment spec** (`deployment_spec.go`) is unchanged: `DeploymentKnowledge.Persistent` and `DeploymentModel.Persistent` remain as implementation-internal fields populated by the normalizer. This isolates the change to the parse/derive boundary — the k8s applier, editability checks, and orphan cleanup keep working off the same internal signal.

**Why not add a `Stateful bool` to `BuiltinProvider`:** every self-hosted knowledge provider today has a mount path, and a non-stateful registered provider has no concrete use case. Adding the field now is speculative complexity. If a future provider needs ephemeral semantics, add `Stateful` then.

**Schema:** the JSON schema (`astropods.schema.json`) drops `persistent` properties from `knowledge`, `knowledge.container`, `integrations.container`, and the agent container block. `additionalProperties: false` is preserved.

**TUI / scaffolding:**
- `ast add knowledge` drops the "Persistent storage?" screen entirely; knowledge entries now go Name → Confirm.
- The scaffold template and `agent_instructions.md` no longer emit or document `persistent:`.

## Migration

For existing `astropods.yml` files:

- `persistent: true` — delete the line. Behavior is unchanged: the provider's mount path or `container.volume` is already the signal.
- `persistent: false` on a provider with a mount path — previously gave an ephemeral qdrant/postgres/etc. There is no replacement; if a user genuinely wants an ephemeral instance of a stateful image, they should use container mode without a `volume` field.
- Specs that omit `persistent` entirely — no action.

The YAML and JSON parsers silently ignore the removed `persistent` field, so `ast push` continues to work against existing specs. `ast spec validate` will report a schema error for any spec that still includes `persistent:` (the schema has `additionalProperties: false`); users running `validate` in CI should remove the field.

**Server-side editability:** the "persistent cannot change after deploy" rule continues to apply against the normalized `DeploymentKnowledge.Persistent`. Practically, this means changing a provider from one with a mount path to one without (or adding/removing `volume` in container mode) is rejected on edit, same as today.
