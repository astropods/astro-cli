# Open (no-OIDC) ingress cohort for agent web surfaces

## Summary

The front-door ALB migration made `*.agents.<domain>` uniformly OIDC-gated: every
agent web surface forces an Astro sign-in. That left no way to publish an agent
reachable without an account — the `anyone` authorization grant relaxed only the
in-cluster authz, not the ALB gate, so an "open" agent was still blocked at the
door.

astro-infra added a second subdomain cohort, `agents.public.<domain>`, whose hosts
fall through the ALB's `*.agents.<domain>` OIDC rule to the no-auth default action
(ALB rule wildcards span dots, so a `.public.<domain>` host never matches). This
change lets astro-server emit hosts in that cohort, per web surface, controlled
from the deploy form.

## Design

The cohort is selected by the host astro-server emits — same Contour/Envoy fleet,
same ingress class, different parent domain. The selector is one config value,
`AGENT_INGRESS_PUBLIC_DOMAIN` (e.g. `agents.public.astropods.ai`), wired through
the astro-server chart; unset disables the cohort.

Two independent web surfaces can opt in, both under `interfaces.auth`:

- `interfaces.auth.web.public` — the messaging web (chat) ingress
- `interfaces.auth.custom` `{ public, grants }` — the agent's own interface (the
  UI/API it serves itself)

Each web ingress resolves its domain through one selector (a live path in the
applier and a mirror in the deployment-record writer, kept in lockstep so the
persisted hostname matches the cluster): `public → AGENT_INGRESS_PUBLIC_DOMAIN,
else INGRESS_DOMAIN`. Slack is unaffected (no HTTP ingress); ingestion is unchanged.

Custom-interface auth lives under `interfaces.auth` rather than on the endpoint so
it reuses the existing template request/sign/deploy channel. A custom-only agent
has no messaging block, so shaping creates an `interfaces` block to carry the auth;
the messaging sidecar stays gated on a non-empty adapter list, so nothing spins up.
Client messaging support is keyed off the sidecar image (not the presence of an
`interfaces` block) so the synthesized block doesn't trip the chat UI.

Custom grants are recorded under a `custom` authorization adapter but are **not**
enforced by the platform today — the agent's own server authorizes. The grants
editor is hidden for the custom interface to avoid implying otherwise; only the
protect/unprotect toggle is surfaced.

**Validation.** A public messaging-web surface bypasses the ALB, so no OIDC
identity reaches the web adapter — org/user grants would lock it out. Deploy
rejects `interfaces.auth.web.public` unless the web grants are `anyone`-only.

**UI.** Both surfaces present a "Protected" toggle (on by default). Turning
protection off warns that anyone with the link can reach the surface without an
Astro account.

## Migration

None required. With `AGENT_INGRESS_PUBLIC_DOMAIN` set and the surfaces defaulting
to protected, existing agents keep their authenticated hosts. Unprotecting a
surface moves only that surface to the open cohort on its next deploy (its URL
changes; the old host still resolves, just forces sign-in).
