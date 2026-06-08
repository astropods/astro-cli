## Summary

External-facing docs for the messaging SDK and adapter packages were missing — users had no place to learn how to wire an agent to the messaging sidecar in either Node or Python. This PR adds nine new pages under Guides covering both layers.

## Design

Two new collapsible subsections under **Guides** in the Fern site:

**Guides → Messaging SDK** — covers the low-level gRPC protocol, intended for custom adapter authors and consumers of `@astropods/messaging` / `astropods-messaging` directly. One page per language (camelCase fields for Node, snake_case for Python). Every proto message and enum is in a reference table, plus worked Slack and web examples in each language.

**Guides → Adapters** — covers the framework-adapter layer most users will actually touch. Five pages:

- Overview with a Mermaid architecture diagram and a package decision table.
- Per-framework quickstarts for Mastra (Node) and LangChain (Python), including their chunk → hook translation tables.
- Custom-adapter guides for both Node and Python documenting the `AgentAdapter` interface, `StreamHooks` lifecycle, `StreamOptions`, `FeedbackEvent` kind taxonomy, and audio handling. Each has a Mermaid lifecycle sequence diagram.

Editorial pass at the end removed internal jargon (Insights buckets, Langfuse attribute names, bridge implementation asides) so the pages read as public-facing reference.

## Migration

None. Docs-only addition. Preview locally with `cd docs-public/fern && fern docs dev`.
