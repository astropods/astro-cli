# Deploy form respects the agent's declared interfaces

## Summary

The deploy form always showed the Slack/Web adapter picker, even for agents that ship their own web frontend and don't run the messaging sidecar. For those agents there's nothing for the user to pick — selecting "web" or "slack" is meaningless because the deployment has no messaging interface at all. This change reads what the agent actually declares and only shows the picker when the agent supports messaging adapters.

## Design

**Signal: the deployment-level `interfaces` block.** The server already emits the `DeploymentInterfaces` block only when the agent spec has `interfaces.messaging: true` (or omits `interfaces` entirely — the legacy default). For custom-frontend-only agents (`interfaces.frontend: true, messaging: false`), `TemplateResponse.template.interfaces` is absent from the wire. The client uses presence/absence of that field as the authoritative signal — no new API surface required.

**`messagingSupported` flag in `useDeployForm`.** Derived as `template?.interfaces != null` and exposed on the hook return. It gates three things:

- The "Chat interface" `FormSection` renders only when true.
- The `errors.adapters = "Select at least one messaging type"` validation is skipped when false.
- `trySubmit`'s `hasAdapter` check passes through when false, so a custom-frontend agent can deploy with an empty `selectedAdapters`.

**Defaults seed empty for custom-frontend agents.** `computeFormDefaults` and `computeInitialValues` previously fell back to `selectedAdapters: ["web"]` whenever the template response had no adapters. They now only do that when the template has an `interfaces` block; otherwise they seed `[]`, matching the fact that no picker will ever be shown.

The picker component (`InterfacesPicker`) and the adapter wire format are unchanged — when messaging *is* supported, the existing Slack + Web options render as before.

## Migration

None. Existing agents that declared `interfaces.messaging` (or omitted `interfaces` entirely) continue to see the picker exactly as today. The only behavioral change is that custom-frontend-only agents no longer see a meaningless adapter section in the deploy and configure flows.
