# Memory Box retrospective on Astropods platform gaps

## Summary

Adds a retrospective of building the Memory Box agent on Astropods as `docs/07-feedback/MEMORY-BOX-LEARNINGS.md`. The goal is to capture, in one place, the expectations an agent-builder forms from the spec and schema — and where the platform's capability or documentation leaves those expectations unmet — so the team has a triageable backlog when evolving the platform.

## Design

The document is organized around a single question: *what did the agent expect to be able to do, and where did reality get in the way?* Every gap is tagged by type:

- **CAPABILITY** — platform doesn't do this yet; needs code.
- **DOCS** — platform does do it, but nothing tells you so.
- **DESIGN** — platform deliberately doesn't do this; the expectation or the schema's shape is what needs to change.

Each §4 item leads with **Expected** / **Reality** before root cause and fix, so the reader can read the diagnosis without already knowing the code. §6 proposes generalizable primitives (connection-string injection, explicit `uses`, S3/embedding providers, MCP interface). §8 lists small, bounded engineering fixes against current `builder.go` and `envresolver.go` line numbers — the highest-impact being the §8.3 parity gap where custom knowledge containers get env injection in deploy but not in `ast dev`.

Claims in the doc have been verified against the current codebase. Notes which items have since shipped (`fea6a941` volumes, `b34c226c` inputs injection) and aligns recommendations with already-made team decisions (e.g. `container.Environment` → `inputs`, `interfaces.auth.web` at deployment-template level).

## Migration

None. Docs-only.
