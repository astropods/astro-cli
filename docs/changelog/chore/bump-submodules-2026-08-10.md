# Bump vendored submodules to latest

## Summary

Advance the pinned submodule pointers to their current remote HEADs so the
monorepo builds against the latest agent-runtime, messaging, adapter, spec, and
marketing-site code. Five submodules moved; astro-cli, astro-infra, and blog
were already current.

## Design

Bumped via `scripts/update-submodules.sh --latest`, which advances each
submodule to its remote branch HEAD and records only the paths that moved. The
new commits converge on three themes:

- **AgentCore runtime** — astro-spec adds `meta.agentcore` and models the
  runtime as `agent.annotations.runtime` (dropping the `IsAgentCore` helper for
  `Container.Runtime()`); adapters gain an AgentCore serving mode; messaging
  adds an AgentCore invoke-per-turn transport. Spec, adapter, and messaging move
  together so the runtime switch is consistent end to end.
- **Multimodal input** — adapters forward inbound images to the model
  (`StreamOptions.images` + Mastra multimodal); messaging adds inline image
  attachments and thread summaries on in-thread Slack mentions.
- **Marketing site** — website replaces the demo calendar link with a gated
  request form served same-origin under `/site-api`, sends lead-notification
  email in the background, and adds hosted email/avatar assets.

Pointer moves (recorded → new):

| Submodule | From | To | Note |
|-----------|------|----|------|
| modules/adapters | 7fc3af7 | b30fb36 | adapter-ai-sdk 0.3.3 → 0.3.5 |
| modules/agents | e9ab089 | 5127369 | spec drift + defect fixes |
| modules/messaging | 4002d82 | c2db753 | v0.0.5-32 → -38 |
| modules/website | d2defe1 | 3c8c69f | marketing forms + assets |
| packages/astro-spec | 07a85f3 | 804cd62 | v0.2.0-1 → -4 |

Bump tooling is hardened in the same change: `scripts/update-submodules.sh` now
stages only the paths that actually moved — never `git add modules`, which swept
stray on-disk repos in as phantom gitlinks — and warns about orphaned checkouts
not declared in `.gitmodules`.

## Migration

None. Consumers pick up the new commits on the next
`git submodule update --init --recursive` (or `scripts/update-submodules.sh`).
