# Machine credentials

## Summary

Every credential Astro accepted belonged to a person. `RequireAuth` takes a
WorkOS user access token or a sealed session cookie, and the only non-human
credential is the per-deployment deploy token, which is minted by the platform
and accepted on two routes. A customer system that needs to call Astro on a
schedule had one option: carry a human's refresh token forever. That attributes
every write to that person, hands the system their full permissions on every
account they belong to, binds it to no account, and breaks silently when they
are offboarded.

This adds **apps**: an account-scoped machine credential, backed by a WorkOS
Connect M2M application, created and managed from organization settings.

The design is specified in
[docs/01-spec/machine-credentials-spec.md](../../01-spec/machine-credentials-spec.md).
This change lands the object, its lifecycle, and the middleware that accepts its
token.

## Design

### WorkOS owns the credential

An app is one row in `account_apps` per WorkOS M2M application. Astro stores the
application ID, the `client_id`, the granted scopes, and a name. It stores no
secret material, so those rows leaking exposes nothing: `client_id` is public by
design.

WorkOS turned out to own more than the secret. `ListApplicationClientSecrets`
returns each credential's ID, hint, and last-used time, which removed a table
from the original design. There is nothing about a credential worth copying
locally, so the credential list is read from WorkOS on demand rather than
mirrored into a schema that could drift.

### Creation order, and what a failure leaves

The WorkOS application is created before the row, so a crash in between leaves an
orphan rather than a row promising access that WorkOS cannot honor. An orphan is
inert, because authorization resolves a client through the row. If the row write
fails, the application is deleted on the way out.

The plaintext secret is returned once, in the creation response, and is never
retrievable. The UI reflects that with a dialog that has to be dismissed
deliberately.

### Scopes

Machine scopes are a separate vocabulary from the human role permissions, so a
scope can never satisfy a role check:

| Scope | Allows |
| --- | --- |
| `members:read` | Read the account's members |
| `audiences:read` | Read audiences and their membership |
| `audiences:manage` | Add and remove audience members |
| `slack_identities:manage` | Record which Slack user is which person |

The vocabulary is WorkOS's, not a list maintained here. A Connect application's
scopes are permission slugs, so the create form reads the environment's
permissions from `GET /authorization/permissions` and offers those. Creation
validates the requested scopes against that same call, so a slug WorkOS does not
know never reaches application creation, and the picker cannot drift from what
is grantable.

System permissions are excluded: WorkOS owns those and they describe its own
surface, so granting one to an app would say nothing about access to Astro.
Filtering where they are read means the picker and the create-time check drop
them together.

An environment with no permissions configured therefore offers nothing to pick,
and an app created without scopes is refused by every scoped endpoint. That is
the honest state rather than a hardcoded list implying more than exists.
`audiences:manage` and `slack_identities:manage` are separate on purpose: a
system that governs access should not also be able to assert who someone is.

Authorization reads scopes from the row rather than from the token, so a scope
change takes effect immediately instead of at the next expiry. They are also
handed to WorkOS on create, so the token carries them too, which a connector
inspecting its own grant can rely on.

### Rotation

WorkOS allows five credentials per application, which makes rotation a two-step
with no downtime: add a secret, move the caller, revoke the old one. Revoking the
*last* secret is refused, because an app with none is a row that looks alive and
cannot be used. Deleting the app is the way to revoke everything, and it deletes
the WorkOS application, which is what actually withdraws access.

The secret list shows each credential's last-used time, since a credential that
never expires is otherwise impossible to retire with confidence.

### Organizations only

A WorkOS M2M application is bound to an organization, and a personal account has
none, so apps are unavailable there and the API says so rather than failing
obscurely. That is the same constraint access groups and fine-grained access
already carry.

### Accepting the token

Validation was never the missing piece. `RequireAuth` verifies any WorkOS JWT
against JWKS, and a machine token is signed by the same keys with the same
issuer, so it already validated. What was wrong is what the middleware built
from it: a `user` whose ID was actually a client ID, and an empty permission set,
because machine tokens carry scopes rather than WorkOS permissions. The result
failed closed on every account route, but it presented a machine as a person to
anything reading the user directly.

`authenticateWithToken` now discriminates first. A machine token names its own
client in both `aud` and `sub`, and a WorkOS user access token carries no `aud`
at all, so the two are told apart before either is trusted. A machine token
resolves its client to an app row, sets an app on the context and deliberately no
user, and fills the session's permissions from the app's scopes. A client with no
row is denied, which is what makes deleting an app revoke its tokens before they
expire.

Because scopes land in the same `Permissions` field the role path already reads,
`RequireAccountPermission` needed one branch rather than a parallel
authorization path: an app satisfies a route by holding the scope the route
declares, on the account the route names. `RequireAccountMember` refuses apps
outright, so membership can never stand in for a scope.

## Migration

Nothing to do. `account_apps` starts empty and no existing path changes. The
feature is inert until an organization creates an app, and the API reports apps
unavailable when no WorkOS API key is configured.
