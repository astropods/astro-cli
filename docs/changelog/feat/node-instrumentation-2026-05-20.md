# Node network interceptor — spec

## Summary

Today, Node agents emit observability only when the developer manually calls `setupObservability(agent)` from `@astropods/adapter-mastra` — not zero-touch, with coverage limited to Mastra's instrumented surface and no way to *modify* outbound traffic (e.g., to route LLM calls through an Astro-managed AI proxy). Beyla's eBPF DaemonSet sees network metadata but cannot decrypt TLS or extract token counts.

This change adds a spec for a zero-touch network-layer interceptor for Node agents. The design artifact only — no code changes.

## Design

Two pieces:

**Interceptor module.** A single bundled CJS file preloaded via `NODE_OPTIONS=--require=/opt/astro/interceptor.cjs` plus a static Go entrypoint wrapper. The interceptor patches `undici` (global dispatcher + module-load hook) and `http`/`https.request`, matches outbound requests against a static rule set, rewrites LLM provider traffic to point at the Astro AI proxy with attribution headers, and taps responses for `gen_ai` usage (Anthropic + OpenAI, streaming and non-streaming). Only matched requests emit spans; non-LLM traffic is Beyla's domain.

**Registry-mirror delivery — the only delivery mechanism.** `astro-registry` serves pre-patched versions of common public Node base images (`library/node:18`/`:20`/`:22`/`:24` × `alpine`/`slim`/`bookworm`) under their canonical Docker Hub paths. When the user writes `FROM node:20`, the pull resolves through our registry and they get the instrumented version transparently — Dockerfile unchanged. Cloud builds get this via BuildKit native registry-mirror configuration; CLI builds via in-process pull-and-tag in `prePullBaseImages`. Agents using non-mirrored bases (`FROM ubuntu`, distroless, custom corporate registries, subversion pins, pre-built `agent.image:` external references) are not instrumented. Coverage expands by adding tags to the mirror, not by adding code.

**Opt-out.** A new optional `agent.instrumentation` field in `astropods.yml`. Polymorphic: bare `true`/`false` for the common case, object form (with `enabled` today, more knobs later) reserved for v2+. `false` skips the CLI's pull redirection — user gets the upstream image and an uninstrumented agent.

Key decisions:

- **No per-request silent fallback when the AI proxy fails.** Rejects bypassing centrally-managed policy (quotas, content rules) and masking proxy problems. Operator mitigation is to unset `ASTRO_AI_PROXY_URL` cluster-wide and roll restart; live-reload is v2.
- **Mirror-only delivery.** Earlier drafts paired a post-build decoration fallback alongside the mirror. Removed in favor of a single mechanism with a clear coverage boundary; long-tail coverage is a CI/config exercise (more tags) rather than additional code.
- **Static Go wrapper, not a shell script.** Works identically on distroless / scratch / shell-bearing images; argv parsing (tini handling, `sh -c` scanning, `node`/`nodejs` basename matching) is unit-testable.

## Migration

Nothing for users until the implementation ships. When it does:

- Existing agents don't change. The interceptor takes effect on the *next* deploy after the feature lands, when the user's build pulls a mirrored base for the first time.
- The `setupObservability()` adapter path goes through a three-release deprecation: detect-and-no-op, deprecation warning, removal.
- `agent.instrumentation` is optional and defaults to on. Users who want to opt out (signed images, FIPS-validated runtimes, agents on non-Node bases who don't need the policy clarity) add `agent.instrumentation: false`.
