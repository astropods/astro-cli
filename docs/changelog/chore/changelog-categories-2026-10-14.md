# Changelog categories

## Summary

Sam wants to know why a CLI command changed. astro-cli had no changelogs, so the reason lived only in commit messages and PR threads. Now every PR adds one, in a folder named for the type of change, such as `docs/changelog/feat/`.

## Design

`CLAUDE.md` now has a Changelogs rule, the same as the astro repo. An entry goes to `docs/changelog/{category}/{short-name}-YYYY-MM-DD.md`. The category is one of `feat`, `fix`, `perf`, `refactor`, `chore`, `docs`, `test`, `ci`, `security` or `revert`, and is never a person, a coding agent, or a branch prefix.

## Migration

None. New PRs add a changelog at that path.
