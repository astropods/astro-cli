# Gateway model selection as first-class config

## Summary

Agents that declare a `provider: gateway` model with multiple options previously
surfaced the deploy-time model picker as a synthesized `MODEL_*` variable. It
landed in the deploy form's "Optional credentials" section — mislabeled (a model
choice is neither a credential nor optional) and grouped with unrelated secrets.
This makes the model choice first-class config, rendered in its own section and
selected by default.

## Design

The model choice is no longer a variable and does not touch the deployment spec
schema. It is a template-layer concern end to end:

- **Generation** injects the default identifier as a literal into
  `agent.environment` (the same mechanism model/knowledge/tool inputs already
  use). This literal is the only thing persisted in the deployment spec.
- **`GatewayModelSelections`** derives the selectors (name, env var, options,
  default) from the agent spec. They are carried through the template layer only:
  cached alongside the base template, passed into `ShapeTemplate` via
  `ShapeOptions`, and surfaced to the UI at the `TemplateResponse.Models`
  response root — a peer of `variables`/`interfaces`/`schedules`/`provisioning`.
- **The choice round-trips** via a new `models` request field (selection name →
  chosen identifier), validated against options and written into
  `agent.environment`. On Configure, the prior choice is restored by folding the
  persisted env literal back into the request, mirroring how stored variables are
  restored.

The client renders a dedicated **Model** section (a real dropdown, placed right
after General), tracks edits locally, and submits them on reshape and finalize so
the selection survives both.

## Migration

None. Existing deployments keep their injected model env value; the picker simply
renders in its own section instead of under "Optional credentials".
