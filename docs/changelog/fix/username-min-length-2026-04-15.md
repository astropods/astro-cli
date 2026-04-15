## Summary

Username validation on the onboarding page showed conflicting error messages: one character triggered a client-side "Must be at least 2 characters" error, then two characters triggered a server-side "account name must be at least 4 characters" error. The client minimum (2) was out of sync with the server's actual minimum (4).

## Design

The `validateAccountName` and `useAccountNameValidation` functions in `use-account-name.ts` defaulted to `minLength=2`. The server enforces a minimum of 4 characters. The `OrganizationNew` page already worked around this by passing `4` explicitly; the onboarding page relied on the default and hit the mismatch.

The fix changes the default from `2` to `4`, aligning client and server validation so users see a single consistent error and the server rejection never surfaces during normal typing.

## Migration

No action required.
