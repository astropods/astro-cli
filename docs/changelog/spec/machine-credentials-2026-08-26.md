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
This change lands the object and its lifecycle. Accepting an app's token on the
API is the next phase and is not here, so an app can be created today but its
token is not yet honored anywhere.

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

They are validated against that list before anything reaches WorkOS, and passed
through to the application so WorkOS puts them in the token. `audiences:manage`
and `slack_identities:manage` are separate on purpose: a system that governs
access should not also be able to assert who someone is.

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

## Migration

Nothing to do. `account_apps` starts empty and no existing path changes. The
feature is inert until an organization creates an app, and the API reports apps
unavailable when no WorkOS API key is configured.
