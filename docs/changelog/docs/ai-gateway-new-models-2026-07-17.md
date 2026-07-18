# AI Gateway public docs: new models + corrected, expanded examples

## Summary

The AI gateway now serves five more Bedrock models beyond the original Claude + Titan
set, and (post-Bifrost migration) the OpenAI-compatible API is served under `/v1` — but
the public docs listed only the Claude models and showed `base_url` set to the bare
`ASTRO_GATEWAY_URL`, which now returns `404`. This reworks the AI gateway page to be
accurate and complete.

## Design

- **Corrected base URL.** All SDK/curl examples now use `${ASTRO_GATEWAY_URL}/v1`; the
  env var is the host only. A callout and an errors table call this out explicitly.
- **Supported models table** adds `nova-pro`, `nova-lite`, `nova-micro` (Amazon Nova),
  `mistral-large-3`, and `pixtral-large` (Mistral).
- **New sections with verified examples:** streaming (`stream: true`), embeddings
  (`titan-embed-text-v2`, 1024-dim), image input (vision via `pixtral-large`, base64
  data URI), structured output, and an errors/limits table (401/404/429).
- **Caveat corrections:** the gateway is no longer Claude-only; image *input* is
  supported (only image *generation* is not).

## Migration

None. Documentation only — the models and `/v1` endpoint are already live. Existing
agents that pointed a client at the bare `ASTRO_GATEWAY_URL` should append `/v1`.
