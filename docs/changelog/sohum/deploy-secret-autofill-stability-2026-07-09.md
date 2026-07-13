## Summary

Deploy form secret auto-fill could use stale vault data during initial load or account switches, which made API key fields appear auto-filled from the wrong source until refresh.

## Design

Vault entries are now exposed to deploy form fields only after the selected account's variables query has real data, not placeholder data from a previous account. Secret inputs also carry stable field names and use `new-password` autocomplete hints so browser password managers can distinguish API-token fields more reliably.

Vault reference validation follows the same readiness boundary. While the user switches the target account and the next account's variables are still placeholder data, the form hides invalid-reference chips instead of validating against the previous account or an empty set. Submission is still blocked whenever the form contains a vault reference and the selected account's variables are not ready, so a transient loading window cannot send a deploy with a secret reference that may not exist for the new target account. Once the selected account's real variables arrive, the usual invalid-reference chips and submit validation resume against that account's names.

## Migration

No user action is required.
