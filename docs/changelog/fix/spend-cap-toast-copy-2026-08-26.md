# Name the support address when a spend limit hits the self-serve ceiling

## Summary

Setting a spend limit above $1,000 showed this toast:

> a spend threshold cannot exceed $1000 per month; contact support about an
> enterprise plan to raise it

Lowercase, no full stop, and "contact support" named no address. The reader is
told to do something and not told how.

The cause is that the toast renders the server's error string verbatim:
`toast.error(getApiErrorMessage(err, "Could not save limits."))`. Those strings
were written in Go style, lowercase and unpunctuated, which is right for a Go
`error` value and wrong for a sentence a customer reads. The ceiling message was
not alone. Every threshold error from the same handler reached the same toast the
same way.

## Design

**The server strings are product copy, so they read like it.** The four
user-facing threshold messages are now sentence case, and the ceiling one names
`support@astropods.com`. That address is already what the public docs send people
to, and it now lives in one place, `billing.SupportEmail`, rather than being
retyped. The CLI and any direct API caller get the same improvement, since they
surface the same string.

**The address is clickable.** `linkifyEmail` turns any address inside a message
into a `mailto:` link, and the limits dialog runs its error toast through it.
Matching on the message would tie the client to one error's wording; finding an
address works for any message that names one, and a message with none is returned
untouched, so existing toasts are unchanged.

Client-side validation would be better still: nothing stops the field accepting
$5,000 and learning the ceiling only from a rejected save. That needs the ceiling
duplicated in the client, so it is left out here.

## Migration

None. Copy only.
