# The session claim decides the account scope, not the cookie

## Summary

The dashboard reads one account at a time, resolved from the
`astro:active-account` cookie. The session JWT carries the organization the
server will actually honor, and the two drift apart, so loaders could scope a
page to an account the session cannot read. Every account-scoped route then
refuses the request.

Four client callers re-scope the session: the account switcher, the deploy
vault, blueprint creation, and org settings. Only the switcher moves the cookie.
The cookie lasts a year, outliving the session it was written for, and a fresh
login scopes to the personal organization while the cookie still names an org.

## Design

`resolveActiveAccount` compares the cookie's account to `organization_id` from
`/me` and drops a cookie the session cannot back. Both places that derive the
scope call it: `getActiveAccount`, which every account-scoped loader uses, and
the root loader, which seeds the switcher.

Reconciling on the server per request rather than in `setActiveAccount` matters
because a click is not the only way the session moves. Any caller can re-scope
it, and the next request follows.

Two cases stay with the cookie: a session claiming no organization, and an
account with no organization of its own. Neither can be shown to disagree.

## Migration

None. A cookie the session cannot back resolves to the account the session is
scoped to, and the switcher shows that account.
