# Fix: AI gateway dev-key rotation no longer collides on the virtual-key name

## Summary

Generating an `ast dev` key intermittently failed with a Bifrost `HTTP 409:
"A virtual key with this name already exists"`, surfaced to the CLI as a 502.
Dev keys are minted per `(account, user)` but were all named `dev/<accountID>`
— a single fixed name per account. Bifrost enforces unique virtual-key names,
so the name collided in two situations, one of them guaranteed:

- **Rotation.** The local reuse window (24h) is deliberately shorter than the
  upstream key TTL (48h) so we rotate before expiry. On rotation we re-mint
  while the predecessor virtual key is still alive upstream (it's deleted only
  best-effort, *after* the new mint), so the re-created name always collided.
- **Cross-user.** Two users on the same account both wanted `dev/<accountID>`;
  the second 409'd.

## Design

The dev-key virtual-key name is now unique per mint:
`dev/<accountID>/<actorUserID>/<nonce>`, where the nonce is a short `uuid`
suffix. The actor id separates users on one account; the nonce separates
successive rotations for a single user. This removes the collision structurally
rather than reacting to the 409, and preserves the existing soft-rotation
behavior — the predecessor key stays valid until its upstream TTL, giving an
overlap window for in-flight dev sessions instead of a hard cutover. Orphaned
keys are reaped by that TTL plus the best-effort delete-of-predecessor already
in `EnsureDevKey`.

Deployment keys are unchanged: their name (`<accountID>/deployment:<id>`) is
already unique per deployment and they never re-mint, so no nonce is added.

## Migration

None. Existing dev keys keep working until their TTL; the next mint uses the new
name.
