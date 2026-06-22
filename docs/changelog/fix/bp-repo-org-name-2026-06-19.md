# Connected repos dropdown shows the owner for non-personal repos

## Summary
In the blueprint create flow, the connected-repository dropdown rendered each entry
with only the repo name. Org-owned repos were indistinguishable from repos in the
user's personal account, so every item looked like it lived under the personal
account until selected.

## Design
The repo list already returns `full_name` (`owner/repo`), and `RepoPicker` already
receives the authenticated personal `githubLogin`. A `repoPickerLabel` helper in
`github-utils` derives the dropdown label from these two values: when the owner
matches the personal login (case-insensitive) it shows just the repo name; for any
other owner (an organization) it prefixes the owner as `owner/repo`. The picker now
renders that label instead of slicing the repo name out of `full_name`.

Keeping the rule in a pure helper makes the personal-vs-org distinction unit-testable
and reuses the data the list query already provides — no new fields or requests.

## Migration
None. Display-only change.
