## Summary

The organization creation endpoint accepted account names that violated the platform's own naming rules — names that were too short, started with a digit, contained uppercase letters, ended with a hyphen, or had consecutive hyphens. The server had a `ValidateAccountName` function that enforces all of these rules, but the `CreateAccount` handler never called it. Only `CheckAccountNameRestricted` was called, which only blocks reserved and brand-protected names.

## Design

`ValidateAccountName` is now called in `CreateAccount` before `CheckAccountNameRestricted`. An invalid name returns HTTP 400 with a description of the violated rule, consistent with how other validation errors are reported.

On the client, `validateAccountName` was already enforcing the same rules via the live input validation, but pressing Enter with an empty or invalid username silently no-oped — the form just did nothing. The submit handler now calls `validateAccountName` directly and surfaces the error message in the form so the user understands why submission failed.

## Migration

No action required.
