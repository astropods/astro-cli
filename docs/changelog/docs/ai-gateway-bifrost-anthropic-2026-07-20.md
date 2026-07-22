# AI Gateway docs: native Anthropic path (Claude Code, Claude Agent SDK)

## Summary

Adds a "Native Anthropic API" section to the AI Gateway docs, covering how Claude Code / Claude Agent SDK agents call the gateway — the one client type that can't use the OpenAI-compatible surface. (The OpenAI-compatible `/v1` base-URL fix and expanded examples landed separately in #1669; this builds on that.)

## Design

The gateway (Bifrost, Bedrock-backed) exposes an `/anthropic` passthrough alongside the OpenAI-compatible `/v1` API. A native Anthropic client reaches it with three gateway-specific details, all verified against a working Claude Code integration:

- **Base URL** `${ASTRO_GATEWAY_URL}/anthropic` — the client appends `/v1/messages`.
- **Auth** via the `x-bf-vk` header — the gateway reads the virtual key there and ignores `Authorization` / `x-api-key`. So auth differs by endpoint: `/v1` uses a bearer token, `/anthropic` uses `x-bf-vk`.
- **Model ids** prefixed `bedrock/` (`bedrock/claude-opus-4-8`); bare `claude-*` returns 401. Claude Code's pre-release beta flags must also be disabled (`CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1`), as the Bedrock backend rejects them.

The "What it doesn't cover" section is updated: native Anthropic is now the documented exception to "no provider-native SDKs," and Anthropic server-side tools (`web_search`, `web_fetch`) are called out as unavailable because the backend is Bedrock.

## Migration

None. New documentation only.
