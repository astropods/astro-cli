# Simplify the stuck-deploy banner actions

## Summary

The stuck-deploy banner carried two buttons plus two inline text links ("Why
deploys get stuck" and "Copy fix prompt") sitting side by side, which read as a
noisy cluster of actions and made the primary next step harder to spot. This
trims it to two clear buttons and folds the docs link into the body copy.

## Design

The banner now shows at most two action buttons: a primary (**Rollback** when an
earlier good version exists, otherwise **Pause**) and a secondary **Copy fix
prompt** (shown when the failure carries a fix prompt). "Roll back" was renamed to
the single word "Rollback". The docs link is no longer a standalone action; it is
embedded inline at the end of the banner body ("For more help, see the docs.") so
it reads as part of the explanation rather than a button. Pause remains available
from the deployment actions menu in every state.

The crash-loop guidance copy was also tightened (server-side event humanizing):
"The container keeps starting and exiting, usually because of an invalid start
command, missing secret, or environment variable. Check the pod logs for the
cause."

## Migration

None. Presentation-only change to the existing banner.
