# Add esc-to-quit to interactive `ast account switch`

## Summary

Running `ast account switch` with no argument opens an interactive picker
backed by `huh`. The only way to dismiss it was `ctrl+c`, which felt
unnatural — most other TUIs in the CLI accept `esc` as a cancel. There was
also no visible hint that aborting was an option, so users would either pick
an account they didn't want or kill the terminal.

## Design

Two small changes to `selectAccountInteractive` in
`apps/astro-cli/cmd/account.go`:

- The form's keymap is extended so `Quit` binds both `esc` and `ctrl+c`. The
  default huh keymap binds `ctrl+c` only and provides no help text.
- The `Select` field gains a static description — `"esc to quit without
  changing account"` — rendered under the title.

`runAccountSwitch` now checks for `huh.ErrUserAborted` from the picker and
returns `nil` (silent exit, no account change) instead of propagating it as
a CLI error.

## Migration

None. Pure UX additive — `ctrl+c` still works exactly as before.

## Drive-by

`humanizeKey` in `apps/astro-client/src/components/deploy/VariableField.tsx`
now treats `GRC` as an acronym, so variable names like `GRC_POLICY` render as
`GRC Policy` rather than `Grc Policy`.
