# Skip redundant GitHub-connect click in blueprint create flow

## Summary

In the new-blueprint wizard's "Starting point" step, choosing "Set up with
GitHub" always rendered a "Connect GitHub" button first — even for users (or
orgs) already connected to GitHub. They had to click Connect, wait for the
status round-trip, and only then see the repository list. The intermediate
"connected" state added a click that conveyed no information for the common
case where a connection already exists.

## Design

The wizard now treats the account's existing GitHub connection as a
first-class input rather than something discovered only on button press.

- The source step queries `useGitHubAccountStatus(selectedOrg)` (enabled only
  once the user reaches the source step, so it adds no upfront cost). Its
  `connected` / `github_login` result is folded into derived values
  `isGitHubConnected` and `effectiveGithubLogin`.
- All source-step gating that previously read the local `githubConnected`
  reducer flag now reads `isGitHubConnected` (button visibility, repo-list
  reveal, `RepoPicker.enabled`, and the Create button's disabled state). When
  the account is already connected the Connect button is skipped and the repo
  list loads directly.
- The not-connected path is unchanged: with no stored connection the Connect
  button shows and `handleGitHubConnect` runs the OAuth/Pipes flow as before.
- The OAuth return path now dispatches the connected state and seeds the login
  on mount, so coming back from OAuth also lands directly on the repo list.

## Migration

None. Behavior is identical for users without an existing GitHub connection.
