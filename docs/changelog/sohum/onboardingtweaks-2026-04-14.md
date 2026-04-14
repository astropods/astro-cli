# GitHub Onboarding Tweaks

## Summary

Polishes the GitHub import path in the blueprint creation wizard and the GitHub connection panel on the draft detail page. Fixes an infinite "initializing" state in the wizard, restricts repo selection to repos where the user has admin access, and replaces the generic "push to trigger" placeholder with three distinct visual states that reflect actual build progress.

## Design

**Admin-only repo filter**: GitHub webhook installation requires admin permission on the target repository. `ListRepos` on the server now filters the `/user/repos` response to only include repos where `permissions.admin === true`, so users only see repos they can actually connect.

**Three-state GitHub sidebar** (`ConnectedRepoView`):
- *Waiting* (amber pulsing dot): shown when `status.builds.length === 0` — the webhook is installed but no `astropods.yml` has been detected yet.
- *In-flight* (build rows): shown when builds exist but the latest is not `"registered"` — displays recent build rows with status indicators.
- *Live* (static green dot): shown when the latest build status is `"registered"` — a green dot with "Live" label and the commit message.

These states mirror the container-path experience; the amber → green transition is the signal that the first push with `astropods.yml` was processed.

**Wizard infinite-initializing fix**: `githubLink.mutateAsync` had `.catch(() => {})` removed in a prior cleanup, which caused link failures to bubble up to the outer try/catch and leave the wizard stuck on "publishing". The `.catch(() => {})` is restored specifically on `githubLink.mutateAsync` — link failures are non-fatal (recoverable from the detail page) and must not block wizard advancement.

## Migration

No migration required.
