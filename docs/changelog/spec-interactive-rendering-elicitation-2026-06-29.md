# Interactive Rendering & Elicitation

**Date:** 2026-06-29
**Spec Version:** 0.17 (main), 0.8 (schema)

## Summary

Specs a single primitive, the **Renderable**, that lets a deployed agent ask the user for structured input mid-conversation and render it natively on any surface (web today, Slack and others next). It is framework-agnostic: existing Mastra, LangGraph, Claude Agent SDK, and MCP agents map onto it with little or no Astro-specific code.

## What it unlocks

- **Tool-call permission/approval on every renderable surface.** The headline use case: an agent proposes a tool call and the user approves, edits, or denies it, drawn as an approval card on web and Slack. Approve/edit/deny outcomes are captured as raw data and emitted to the OTel → Langfuse pipeline, so approval and denial rates per tool and per agent can feed model-performance metrics.
- **Custom elicitations.** Any structured ask (text, single/multi-select, confirmation, edit-a-proposal) is described once as JSON Schema and drawn by host-native widgets per surface. A developer can also define a tool that elicits and handle the response in their own deterministic code, so the answer need never return to the model.
- **Durable answers.** A pending ask survives page reload, waits indefinitely while the thread is open, and resumes after an agent restart for checkpointed frameworks.

## Design

- Keeps the existing gRPC messaging SDK as transport and adds an MCP-compatible data model; MCP elicitation is a subset profile, so MCP-native agents pass through unchanged.
- Full JSON Schema (data) with optional inline render hints, so the same Renderable renders across surfaces without a shared renderer. A companion Renderable Schema Specification defines the supported types, fidelity profiles, and validation.
- Correctness does not depend on a held-open connection: a durable interaction row (in the deployment-local store, kept out of the central DB) is the source of truth and delivery is idempotent, so an answer is never lost.

## Migration

None. This adds design specs only; there is no runtime change. Existing agents, deployments, and text chat are unaffected.
