## Summary

Introduces the `ast account` command group as the first step in the noun-verb CLI redesign. Establishes a working account context that scopes all subsequent platform commands to the active account.

## Design

Three subcommands are added under `ast account`:

- `account list` — reads from the credential store and prints all accounts the user belongs to, marking the active one with `✓` and labeling personal accounts with `(personal)`.
- `account switch <name>` — updates the active account in the credential store. Accepts `-` to toggle back to the previous account (mirroring `git switch -`). When invoked with no argument, shows an interactive picker. `ast login` gains an optional `--account <name>` flag that switches immediately after auth.
- `account token` — prints an API token scoped to the active account: personal access token for personal accounts, org-scoped token for org accounts.

Account state is persisted in the credential store via two new `Profile` fields: `CurrentAccount` and `PreviousAccount`. `SetCurrentAccount` validates the target name, records the outgoing account as previous (defaulting to the personal account when none is set), and keeps `switch -` correct even on the first explicit switch.

Two shared helpers (`getCurrentAccountToken`, `getAccountToken`) are introduced in `account.go` for other commands to resolve a scoped token without duplicating auth logic.

`ast whoami` now shows the active account from `GetCurrentAccount`.

## Migration

Nothing required. Existing credentials and the login flow are unchanged.
