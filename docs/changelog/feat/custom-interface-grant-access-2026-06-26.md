# Grant access editor for the custom interface

## Summary

The deploy form's custom-interface section previously exposed only a "Protected"
toggle (no-OIDC ingress cohort vs. open). It now also surfaces the same per-subject
"Grant access" editor used by the messaging web chat, so deployers can record who
(anyone / org / specific users) should reach the agent's own web UI.

## Design

Access is a single "Who has access" dropdown (`CustomAccessControl`) with three
mutually-exclusive modes, rather than a separate "Protected" toggle plus a grants
list:

- **Astro members** — OIDC-gated, `auth.custom.grants = [{ anyone }]`. The
  fresh-deploy default, so the form never starts in a "no one has access" state.
- **Public** — the no-OIDC public ingress cohort (`auth.custom.public = true`);
  shows an open-access warning.
- **Invited only** — OIDC-gated; reveals a per-subject grant list where
  organizations and individual users are added via the member picker and removed
  with a per-row control.

The mode is derived from `auth.custom` state (`public` → public, an `anyone` grant →
anyone, otherwise specific), and switching modes sets `public` + `grants` directly.
The custom interface reuses the shared grant pieces (`GrantRow`, `MemberPicker`,
`AddGrantMenu` with `hideAnyone`); the messaging web/slack `GrantsEditor` is
unchanged apart from an adapter-aware "anyone" label — web and custom render
"Anyone with an Astro account", Slack keeps "Anyone" (workspace-scoped).

Grants are captured and persisted through the existing channel
(`deployment_authorization_grants`, `adapter='custom'`).

## Enforcement (pending BE follow-up)

Platform-level enforcement of custom grants at request time is **not yet live** —
today the front-door ALB only gates signed-in vs. not for the custom ingress, so
org/user grants are recorded but not honored. Enforcing them requires an ext_authz
hop in front of the agent ingress that calls `GET /deployments/authorize` (which
already accepts `adapter=custom`). This is a separate backend/infra change owned by
engineering.

## Migration

None. Existing deployments keep their current access behavior until the BE
enforcement lands.
