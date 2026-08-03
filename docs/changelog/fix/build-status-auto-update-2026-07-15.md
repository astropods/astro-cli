# Fix: deploy page auto-updates when an in-progress build finishes

## Summary

On a GitHub-connected blueprint, the deploy page showed a "Building" card while a
build was in flight, but when the build finished the card vanished and the
finished build did not appear as an available upgrade until a manual page
refresh. Reported from production (issue #1627): a completed build should become
the latest build to upgrade to, not disappear.

## Design

The Deployment History panel drew its two build cards from two different queries
with different freshness:

- the "Building" / "Preparing" card came from `useGitHubStatus`, which self-polls
  every 5s while the latest build is pending or building, and
- the "New build available" nudge came from `useAccountBlueprints`, which did not
  poll at all.

So the instant a build completed, the polling card disappeared while the nudge's
source stayed stale, and the build was lost from view until a refetch.

The fix derives the available build from the freshest poll-backed source: the
newest finished GitHub build (status `registered`) whose build id differs from
the deployed one. Since `useGitHubStatus` polls through the build, the moment the
poll observes completion the finished build becomes the upgrade target, with no
refresh. The blueprint-versions path is kept as a fallback for builds published
outside the connected repo (for example a public cross-account blueprint). A
modest baseline poll (15s) was also added to the panel's status query so a build
pushed while the page is open surfaces on its own, then the existing 5s in-flight
poll catches its completion.

## Consistent build metadata (issue #1629)

The "New build available" nudge showed only a bare commit sha, while the active
deployment tile showed branch plus build id, so the two read differently. Both
now render through a shared `DeploymentSourceLine` component, so the available
build shows its branch and build id in the same format as the active one (the
nudge omits only the relative time, since the build is not deployed yet).

## Build-state design refresh

Design feedback asked the three build states to read more distinctly:

- **Building** now reads as actively running: a slate card with a slight indigo
  band that slides across (a shimmer that fades as it sweeps), alongside the
  spinner and a status line that rotates through "Pushing new build" / "Building
  image" / "Almost there". This separates it from the static active tile beneath
  it.
- **New build available** keeps a small "New build available" eyebrow above the
  commit message so that label is not lost once the branch and build id line is
  shown.
- **Latest build** adds a subtle badge in the panel header when there is no build
  in flight and nothing newer to deploy, so the panel does not simply fall silent
  once the upgrade nudge clears.

## Migration

None. Client-only behavior change to the deploy page.
