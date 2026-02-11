# Project Overview

Astro is a platform for deploying and running AI agents of any kind. It provides agent-native infrastructure including models, knowledge bases, tool integrations, and observability. Agents can be packaged as containers and deployed with declarative configuration.

The project is a monorepo that contains packages for agent deployment infrastructure and optional utilities for building agents.

IMPORTANT: When planning and creating spec, please ensure it's concise and complete while trading off verbosity and grammar. Avoid putting code examples unless necessary.

# Tooling

This is a bun monorepo. Always use `bun x <command>` instead of `npx`.

# Data Fetching (astro-client)

All server data integration uses TanStack Query. See [docs/04-guides/tanstack-query.md](docs/04-guides/tanstack-query.md) for architecture, conventions, and best practices. Key rules:
- Never call `api.*` directly in components for reads — use query hooks from `src/api/queries/`.
- All query keys must come from the factories in `src/api/queries/keys.ts`.
- Mutations invalidate affected queries in `onSuccess`.