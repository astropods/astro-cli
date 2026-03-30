## Summary

Clicking Pause on a deployment and seeing no visible response would leave the button re-enabled before the query refetched. A second click would then hit the server with a 400 because the deployment was already stopped.

## Design

The root cause was a logic error in the `pausing` effect in `ActiveDetailView`. The condition `isPaused || !isDeploying` exited immediately for any `active` deployment — since `active` is not a deploying state, `!isDeploying` was always true. This reset `pausing` to `false` before the mutation resolved, ending the loading/disabled state prematurely.

The fix changes the exit condition to `isPaused` only, so `pausing` stays `true` (and the button stays disabled) until the query reflects the new `stopped` or `scaled_down` status.

## Migration

No action required.
