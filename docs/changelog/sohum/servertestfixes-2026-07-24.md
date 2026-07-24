# Summary

Restore the astro-server test suite after the cross-account deployment tests introduced an account fixture that no longer matched the account store's scan contract.

# Design

The deployment cache tests now construct account query rows through the shared account SQL-mock helper. Keeping the fixture aligned with the account store's canonical column order prevents schema drift from surfacing as misleading account-not-found responses.

# Migration

No migration is required.
