# Supabase as a knowledge store type

## Summary

Users can now add a Supabase database as a knowledge store directly from the
"Add store" provider picker, alongside PostgreSQL, MySQL, Redis, Neo4j, and
Pinecone. Connecting authorizes Astro via Supabase OAuth, lists the account's
projects, and auto-fills the connection details — the user only supplies the
database password.

## Design

**Supabase is a first-class picker entry, but a Postgres store underneath.**
A Supabase store is an ordinary external PostgreSQL connection
(`db.<project>.supabase.co:5432`, database `postgres`, user `postgres`). Rather
than teach every provider-switching code path a new type, `supabase` is a
client-only provider in the picker: selecting it runs the OAuth import flow and
then creates the store with `provider: "postgres"`. The DB `provider` column is
never `supabase`, so deploy/bind/health-check logic is untouched. The trade-off
is that a connected Supabase store is indistinguishable from any other Postgres
store after creation (no per-store Supabase badge).

**OAuth is brokered by WorkOS Pipes as a custom provider.** Rather than
hand-roll the OAuth flow and a token vault, Supabase is registered as a WorkOS
Pipes custom provider (slug `supabase`) — the same mechanism GitHub and Slack
already use. WorkOS hosts the callback, stores and refreshes the access/refresh
tokens, and hands the server a short-lived access token on demand via
`GetAccessToken`. The account-scoped endpoints are thin wrappers over the
`pipes.Client`: connect → `GetAuthorizationURL`, status/projects →
`GetAccessToken`, disconnect → `DeleteConnection`. WorkOS returns the browser to
an authenticated, account-scoped callback (`/api/v1/accounts/:account/supabase/
callback`) that bounces to the frontend — no code exchange, token vault, PKCE
state, or KMS on our side. "Connected" means WorkOS can serve a token (mirrors
GitHub/Slack), so an expired access token still reads as connected because WorkOS
refreshes it. A token WorkOS can't serve, or one Supabase rejects with 401,
surfaces as "not connected". The only server-held Supabase call is
`GET https://api.supabase.com/v1/projects` with the WorkOS-served bearer token.
Config: none beyond the existing `WORKOS_API_KEY`; the Supabase client
credentials live in the WorkOS custom-provider definition.

**Entry points.** Connect/disconnect is available both inline in the add-store
flow and from the Connectors settings page (alongside GitHub and Slack). The
add-store page pre-selects the provider from `?provider=`, so the OAuth
round-trip returns straight to the Supabase form.

**Connection panel UX.** In the add-store flow the Supabase step is a single
self-contained card whose two states share one persistent "Connection" section
header. Before OAuth it is a focused connect card — a primary "Connect Supabase"
action plus a sign-up link, with no store-name or configuration fields, since
there is nothing to configure until projects load. After connecting it becomes a
provider-identity header (icon, "Supabase", connection status) over a project
picker (region tags, "Create new project" link); the database password field and
a read-only host preview appear only once a project is selected, keeping the card
compact until then.

## Migration

None. The integration is optional and off unless the Supabase custom provider is
configured in WorkOS Pipes. No new tables, no KMS, no Supabase-specific env vars.
