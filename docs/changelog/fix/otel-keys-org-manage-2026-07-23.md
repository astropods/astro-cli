# OTel ingest keys require org:manage

## Summary

Managing account-scoped OTel ingest keys (list/create/revoke) previously required the `org:admin` permission, which only the org **owner** holds. The `admin` role was shown the "API Keys" settings panel in the client but received a 403 when creating a key. This change lowers the requirement to `org:manage`, which both `admin` and `owner` hold, aligning enforcement with the existing UI gating.

## Design

The three key routes moved from the `org:admin` route group to the existing `org:manage` group in `astro-server`:

- `GET /accounts/:account/otel-keys`
- `POST /accounts/:account/otel-keys`
- `DELETE /accounts/:account/otel-keys/:tokenID`

Account rename/delete, avatars, and audit log stay on `org:admin`. Personal accounts are unaffected — their single member holds all permissions implicitly.

## Migration

None. Org owners retain access; org admins gain the ability to manage ingest keys they could already see in settings.
