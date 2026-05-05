# Generate blueprint avatar on CLI push (RegisterAgent)

## Summary

Blueprints pushed via the CLI (`ast push`) were missing their generated avatar
because the `RegisterAgent` handler never received avatar-generation wiring.
The client fell back to a static generic placeholder or showed a broken image
link until the 24-hour backfill job ran.

## Design

When `d4540268` moved identity generation from the client-side TypeScript
package to server-side Go + S3 storage, the generation was added to
`CreateBlueprint` (the UI "create shell" path) and a periodic River backfill
job — but `RegisterAgent` (the CLI push path) was missed. Since most blueprints
are created through the CLI, the majority of new blueprints had no avatar on
first load.

The fix adds `avatarStore` to `RegisterAgent`'s parameter list and generates
a deterministic JPEG identity immediately after successful registration, using
the same `identitygen.GenerateIdentityJPEG` + `extractAndStoreColors` pattern
already established in `CreateBlueprint`. An existence check
(`AgentAvatarExists`) prevents overwriting user-uploaded avatars on re-push.
Failures remain non-fatal — the backfill job still acts as a safety net.

## Migration

None required. Existing blueprints that were already backfilled keep their
avatars. Blueprints created before the backfill ran will receive avatars on
their next push or on the next backfill cycle.
