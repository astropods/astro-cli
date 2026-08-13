# Organization fine-grained access opt-in

## Summary

Organization owners and admins can opt their organization into private, resource-specific deployment access from Settings → Experiments. The choice is stored and audited by Astro so it can safely become an authorization boundary; a browser-local experiment flag is not trusted for security.

## Design

The new Fine-grained access switch defaults off. Off keeps current organization-membership behavior authoritative. On makes PR7.1 capability responses use live WorkOS permissions for PR4-synchronized deployments; PR7.3 will use the same gate for mutation enforcement.

Shadow checks remain active independently of the organization choice, so opted-out organizations continue producing comparison evidence. Deployment resource synchronization and creator `deployment-editor` assignment also remain independent. The switch never starts a backfill: historical deployments stay on legacy behavior until the post-PR9 migration converges.

Access is private by default. Owners and admins inherit access to every deployment, creators receive `deployment-editor`, and other members receive no deployment access until a person or group receives a resource role. WorkOS roles remain permission bundles; Astro continues treating the five flat deployment permissions as the client and policy contract.

The rollout is deliberately split:

1. PR7.1 reports effective capabilities without denying existing routes.
2. PR7.2 persists the organization choice and exposes the Experiments control.
3. PR7.3 enforces reviewed mutation routes only when both the global kill switch and organization opt-in are enabled.

## Review plan

- Confirm only organization owners/admins can read or change the setting, and every change writes an audit event.
- Confirm the Experiments navigation sits directly below Audit Log and uses the same switch presentation as personal experiments.
- Confirm a missing setting is off and turning it off returns capability responses to legacy mode.
- Confirm turning it on selects FGA capability mode only for synchronized organization deployments.
- Confirm shadow checks, resource synchronization, and creator assignments do not depend on the organization setting.
- Confirm the switch performs no deployment backfill.

## Preview test

Open Organization Settings → Experiments as an owner/admin and enable Fine-grained access. For a deployment created after PR4, call `GET /api/v1/deployments/:id/capabilities`: assigned users should receive `mode: fga` and their live WorkOS permissions. Disable the switch and repeat: the same request should return `mode: legacy`, while PR6 shadow comparison logs continue appearing. Verify the toggle event in the organization Audit Log.

## Migration

No existing organization is opted in automatically. Historical deployments are not changed by this PR.
