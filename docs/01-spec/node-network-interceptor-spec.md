# Node Network Interceptor Spec

**Version**: 0.1
**Status**: Draft — for review
**Date**: 2026-05-20

## Overview

Astro agents that run on Node.js today must call `setupObservability(agent)` from `@astropods/adapter-mastra` to emit telemetry. This is non-zero-touch — the developer has to know it exists, import it, and wire it before their agent runs. Coverage is also limited to what Mastra itself instruments; raw `fetch` / `undici` / `http` calls outside the Mastra path are invisible.

This spec replaces that with a **zero-touch network-layer interceptor** that:

1. **Is invisible to the developer** — no SDK imports, no env vars to set in user code, no spec field to enable.
2. **Intercepts outgoing HTTP traffic** at the Node networking primitives and can rewrite destination + headers (used to route LLM calls through the Astro AI proxy).
3. **Observes incoming responses** — emits OTel spans for every outbound request and extracts gen_ai usage (token counts) for known providers, including streaming responses.
4. **Works transparently for recognized public Node base images** — `FROM node:20`, `node:20-alpine`, `node:20-slim`, `node:20-bookworm`, and the corresponding `:18` / `:22` / `:24` tags. Agents using non-mirrored bases (`FROM ubuntu`, distroless, custom corporate registries, subversion pins, pre-built `agent.image:` external references) are not instrumented; see §2.6 for the explicit coverage boundary.

The interceptor reaches the agent process through a single mechanism: **base image substitution via a registry mirror**. When the user's Dockerfile uses a recognized public Node base (`FROM node:20`, `node:20-alpine`, etc.), our registry `astro-registry` serves a pre-patched version of that image under the same canonical path that Docker Hub would. The user's Dockerfile is **not modified** — neither on disk nor in memory. The build's pull of `node:20` simply resolves through our registry instead of Docker Hub, and what it gets back is already instrumented. The interceptor is inside the base layer before the user's build even starts. No per-build decoration step, no extra layer, no work on the hot path.

The interceptor module sits at `/opt/astro/interceptor.cjs` in the patched base image; the entrypoint wrapper sits at `/opt/astro/entrypoint`; the wrapper preloads the interceptor via `NODE_OPTIONS=--require=...` when it detects Node at startup.

The flip side of having only one delivery mechanism is a clear coverage boundary: agents that use a base image we don't mirror (custom corporate bases, `FROM ubuntu:22.04` + manual Node install, non-Docker-Hub registries, pinned subversion tags outside the mirror set, pre-built external images referenced via `agent.image:`) **do not receive instrumentation**. This is a deliberate trade for simplicity — a single, well-understood mechanism with a clear "instrumentation requires a supported base image" contract beats two mechanisms that try to cover every case. Coverage expands by adding tags to the mirror set, not by adding code paths.

This spec covers the interceptor module, the registry-mirror delivery mechanism, deployment integration, edge cases, and migration. The AI proxy service that the interceptor forwards to is a separate workstream — only its wire-level contract is specified here.

## Goals

1. **Zero-touch instrumentation.** A developer pushing arbitrary Node code gets full network observability and AI-proxy routing without changing their code, their Dockerfile, or their `astropods.yml`.
2. **Observe and modify.** The interceptor can both record requests (spans, byte counts, token usage) and rewrite them (destination, headers) before they hit the wire. Existing OTel auto-instrumentation can only observe.
3. **Layered patching, fail-open.** Patch at `fetch`, `undici` Dispatcher, and `http` / `https` simultaneously so a single library quirk does not produce a coverage gap. If any patch fails, the agent runs without that coverage rather than crashing.
4. **Respect the user's runtime.** Honor existing `NODE_OPTIONS`, the user's original `ENTRYPOINT` / `CMD`, native modules, and read-only root filesystems. The patched base image must be a pure additive layer on top of upstream.
5. **Single delivery mechanism.** The interceptor reaches the agent process exclusively through base-image substitution at the registry layer. No per-build decoration step, no post-build image transformation, no per-tenant work on the hot path. The constraint this implies — that only agents using mirrored Node bases get instrumented — is accepted in exchange for a single well-understood mechanism with a clean operational boundary.
6. **Friendly opt-out surface.** One spec field — `agent.instrumentation` — controls whether instrumentation is applied. It accepts a bare `false` for "off," a bare `true` (or absence) for "on," or an object for future fine-grained controls. Runtime detection at the wrapper still re-confirms what was decided at build/ingest time, so an enabled-but-ambiguous image never gets broken by mis-injection.
7. **Multi-runtime ready.** v1 implements only the Node interceptor, but the architecture is built so additional runtimes can slot in without redesign. The base-mirror pipeline shape, spec field, AI proxy contract, rule set, OTel pipeline, and kill switches are runtime-neutral by design; only the interceptor module and binary-name detection are per-runtime. See §11 for the extensibility model. Specific patch surfaces and bootstrap mechanisms for future runtimes are intentionally out of scope until that runtime is investigated.

## Non-Goals

1. **Bun, Deno, Python, native binary runtimes.** v1 ships Node only. Agents using other runtimes don't have a corresponding mirror set and are not instrumented. The existing `setupObservability` adapter path remains for Python/Mastra-on-Bun users until those runtimes get their own interceptors.
2. **HTTP/2, gRPC, WebSocket interception.** Most LLM providers use HTTP/1.1 or HTTP/2-via-fetch — both of which bottom out in `undici`. Native HTTP/2 clients (`http2.connect`), `@grpc/grpc-js`, and `ws` are out of scope.
3. **TLS MITM of arbitrary egress.** The interceptor never terminates TLS that was destined elsewhere. Rewriting to the AI proxy works by initiating a *new* TLS connection from inside the agent process; the AI proxy is the user-trusted endpoint. No CA certificates are installed in the user image.
4. **Hostile-tenant containment.** This is observability for the tenant's own benefit, not enforcement against a hostile tenant. A malicious agent can disable the interceptor (delete `/opt/astro`, override `NODE_OPTIONS`, etc.). Beyla covers the kernel-level enforcement path.
5. **Image signature preservation.** Patched base images served by `astro-registry` are republished under our keys, not the upstream's. Users who rely on the upstream Docker Inc. signature on `node:20` will see a different signature when pulling through our registry. This is an accepted consequence of the substitution model.
6. **Synchronous body modification.** The interceptor can rewrite *request* destination and headers, and *observe* response bodies. It cannot synchronously rewrite request or response *bodies* in v1. Body rewriting is the AI proxy's job.
7. **A new Node Dockerfile template.** We don't ship a Node scaffold template. Users continue to bring whatever Dockerfile they want; the base-image substitution mechanism (§2.2) makes their existing `FROM node:20` work transparently.
8. **Instrumentation of non-mirrored bases.** Agents using `FROM ubuntu:22.04` + manual Node install, `FROM gcr.io/distroless/nodejs20`, custom corporate bases, pinned subversion tags outside the mirror set, or pre-built `agent.image:` external images are **not instrumented**. Coverage is bounded by the mirror set. To expand coverage, add tags to the mirror set; we deliberately do not introduce a second delivery mechanism to catch the long tail.

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ Astro base image mirror (built ahead of time, once per release)      │
│                                                                      │
│   upstream: docker.io/library/node:20, node:20-alpine, ...           │
│       │                                                              │
│       ▼  patch pipeline                                              │
│   astro-registry: library/node:20, library/node:20-alpine, ...       │
│       (each tag = upstream + /opt/astro/interceptor.cjs              │
│        + /opt/astro/entrypoint + ENTRYPOINT wrapper)                 │
└──────────────────────────────────────────────────────────────────────┘
                              │ served on demand
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Build / Ingest paths                                                 │
│                                                                      │
│   astro-cli build (local)        ┐  Pulls of recognized Node tags    │
│   cloud builder (GH integration) │  resolve through astro-registry;  │
│                                  │  Dockerfile is unchanged. Build   │
│                                  │  receives the patched base image  │
│                                  │  directly.                        │
│                                                                      │
│   Non-mirrored bases (ubuntu, distroless, custom, pinned subversion, │
│   pre-built agent.image:) are pulled unchanged from upstream and     │
│   produce uninstrumented agents. This is the documented coverage     │
│   boundary; expand by adding tags to the mirror set.                 │
└──────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Pod runtime (instrumented agents only)                               │
│                                                                      │
│   /opt/astro/entrypoint                                              │
│     export NODE_OPTIONS="--require=/opt/astro/interceptor.cjs $..."  │
│     exec <original entrypoint> <original argv>                       │
│           │                                                          │
│           ▼                                                          │
│   node --require=/opt/astro/interceptor.cjs <user code>              │
│           │                                                          │
│           ▼                                                          │
│   interceptor.cjs (loaded before user code)                          │
│     1. install module-load hooks (undici, http, https)               │
│     2. setGlobalDispatcher(AstroDispatcher)                          │
│     3. monkey-patch http.request, https.request                      │
│     4. wire OTel (reuse if user has one, else init ours)             │
│     5. yield                                                         │
│           │                                                          │
│           ▼                                                          │
│   user code runs; matched outbound HTTP requests are observed        │
│   and optionally rewritten to the AI proxy.                          │
└──────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Egress targets                                                       │
│                                                                      │
│   Non-LLM traffic     ──►  upstream directly (no interceptor span)   │
│   LLM traffic         ──►  Astro AI proxy ──► upstream provider      │
│   Span emission       ──►  per-deployment OTel collector             │
└──────────────────────────────────────────────────────────────────────┘
```

## 1. The Interceptor Module (Node)

The interceptor module is the only runtime-specific piece of this design. The rest of the architecture — base-mirror pipeline shape, spec field, AI proxy contract, rule set, OTel pipeline, kill switches — is runtime-agnostic. See §11 for how additional runtimes plug into the same architecture.

### 1.0 Supported Node.js versions

| Version | Status | Notes |
|---|---|---|
| Node 18.x | **Supported (minimum)** | First version that ships `undici` as a built-in. `globalThis.fetch` is the standard Node fetch. |
| Node 20.x LTS | **Supported (primary)** | Tested baseline; CI matrix builds against this. |
| Node 22.x LTS | **Supported** | |
| Node 24.x | **Supported** (current) | |
| Node ≤ 17 | **Unsupported** | No built-in `undici`, no `globalThis.fetch`, EOL. |
| Node ≥ 26 | **Untested** | Should work — the patch surface targets stable APIs — but not in the CI matrix until that version reaches LTS. |

Behavior on **unsupported** Node versions:

- The interceptor's top-level try/catch (§1.2 step 1) catches any "API not found" / "module not exported" error during bootstrap and falls back to no-op. The agent process continues to run; only the interceptor doesn't install patches.
- The entrypoint wrapper logs a single line to stderr at startup when it detects an unsupported Node version: `[astropods-interceptor] unsupported node version <X.Y.Z>; interceptor will run no-op. minimum: 18.x`. The wrapper still injects `NODE_OPTIONS=--require=...` — the interceptor itself decides at bootstrap whether it can run; the wrapper is intentionally not the gatekeeper, because version-string parsing in shell-level argv is brittle (e.g., `nodejs-current`, custom builds with non-standard `--version` output).
- No deploy-time block. The mirror build pipeline doesn't probe Node versions at runtime — that would require running upstream images, which is expensive. The pod boots, interceptor no-ops on unsupported versions, OTel spans are simply absent.

Behavior on **future untested** versions:

- The interceptor's patch installation is per-layer (undici / module-load / http) with each layer in its own try/catch. If undici's internals shift in a way that breaks our `setGlobalDispatcher` subclass, the undici layer fails but the http/https monkey-patch still installs. Spans for `axios` / `got` keep working; only the direct-fetch path is uncovered. A `astropods.interceptor.patch_failures_total{layer="undici_dispatcher"}` counter (§6.3) lights up so we can detect this in the fleet before users do.

Behavior with **Node-compatible non-Node runtimes** (Bun, Deno running in Node-compat mode):

- Out of scope. Bun/Deno/Python images don't pull from the Node mirror, so they aren't patched at all. The interceptor never loads. No crash, just no instrumentation. (For the corner case where a user's mirrored Node base ends up invoking a non-Node binary as its entrypoint, the wrapper's runtime detection in §2.3 catches that and skips injection.)

### 1.1 Distribution

A single npm-style package, `@astropods/node-interceptor`. Bundled into one CommonJS file (`interceptor.cjs`) via esbuild. Pure JavaScript — no native dependencies. The mirror build pipeline copies this file plus a `package.json` stub into `/opt/astro/` of each patched base image.

Two build artefacts:

| Artefact | Purpose |
|---|---|
| `astropods/node-interceptor:vX` (Docker image) | Source for `COPY --from=` in the base-mirror Dockerfile |
| `@astropods/node-interceptor@vX` (internal npm package) | Source of truth; image is built from it |

The Docker image contains only `/interceptor/interceptor.cjs` and `/interceptor/entrypoint` (the wrapper, see §2.4). Multi-arch (`linux/amd64`, `linux/arm64`) — the JS file is the same on both arches; the entrypoint binary differs.

### 1.2 Bootstrap and install order

The interceptor's top-level code (in order) is:

1. Wrap the entire bootstrap in a top-level try/catch. On any exception, log to stderr with the prefix `[astropods-interceptor]` and continue (interceptor effectively no-ops). Never throw — a thrown error from a `--require` module aborts Node startup.
2. Read configuration from environment variables (see §1.7). If `ASTROPODS_INTERCEPTOR_DISABLED=1`, return early.
3. Install module-load hooks via `import-in-the-middle` and `require-in-the-middle` for `undici`, `node:http`, `node:https`, `http`, `https`. The hooks wrap exports as they are loaded by user code, catching `import { fetch } from 'undici'` and similar that capture references before `setGlobalDispatcher`.
4. Lazy-load `undici` (already shipped with Node 18+) and install our `AstroDispatcher` as the global dispatcher via `setGlobalDispatcher`.
5. Monkey-patch `http.request`, `http.get`, `https.request`, `https.get` directly as a fallback for SDKs that bypass `undici`.
6. Initialize the OTel provider:
   - If `@opentelemetry/api`'s global tracer provider is a real provider (not the no-op default), reuse it. The user has their own SDK.
   - Otherwise, initialize an internal OTel NodeSDK with the OTLP HTTP exporter pointing at `OTEL_EXPORTER_OTLP_ENDPOINT`.
7. Register a `process.on('beforeExit')` handler to flush pending spans.
8. Publish a presence marker at `globalThis[Symbol.for("astropods.interceptor")]` — an object containing `{ version, dispatcher: AstroDispatcher }`. Other Astro packages (e.g., the legacy Mastra adapter) read this to detect that the interceptor is active. The Symbol key avoids any namespace pollution; the global object's normal property enumeration does not see it.
9. Yield (top-level returns).

The interceptor adds nothing to `globalThis` other than the Symbol-keyed presence marker above and never holds references to user objects. Its only persistent state is the dispatcher, the OTel provider, and the marker.

### 1.3 Patch surface

Three layers, each catching a slightly different class of caller:

| Layer | Catches | Mechanism |
|---|---|---|
| **undici global dispatcher** | `globalThis.fetch`, and any code using the default undici dispatcher | `undici.setGlobalDispatcher(AstroDispatcher)` — `AstroDispatcher` delegates to the original `Agent` after applying rewrite/observation |
| **Module-load hook on `undici`** | `import { fetch, Agent } from 'undici'` — code that constructs its own Dispatcher | Wrap the named exports at load time so user-constructed `Agent` instances inherit our wrapper |
| **`http` / `https` monkey-patch** | `http.request`, `https.request`, and everything that bottoms out in them (`axios`, `got`, `node-fetch@2`, etc.) | Replace `http.request` / `http.get` / `https.request` / `https.get` with wrappers that observe and (optionally) rewrite |

#### AstroDispatcher

A subclass-by-composition of `undici.Dispatcher`. Wraps an inner `Agent` (the original default dispatcher). On `dispatch(opts, handler)`:

1. Construct an `OutboundRequest` view from `opts.origin`, `opts.path`, `opts.method`, `opts.headers`.
2. Match against the rewrite rule set (see §1.4).
3. If no rule matches: delegate to `innerDispatcher.dispatch(opts, handler)` unchanged. No wrapping, no span — the dispatch overhead is one regex check per request and nothing else.
4. If a rule matched and produced a rewrite, mutate `opts` to point at the new origin/path with merged headers.
5. Wrap `handler` with `AstroHandler`, which forwards every callback to the original handler and additionally:
   - Captures `onConnect`, `onError`, `onResponseStarted` for timing.
   - Captures status code and response headers in `onHeaders`.
   - Tees the response body via `onData`, forwarding bytes to both the original handler and our parser (see §1.5).
   - Finalizes the span in `onComplete`.
6. Delegate to `innerDispatcher.dispatch(opts, wrappedHandler)`.

The Dispatcher implementation never buffers the response body in memory — bytes flow through unmodified to the user's handler. The tap is a parallel parser, capped at a configurable byte limit (`ASTROPODS_INTERCEPTOR_MAX_BODY_PARSE_BYTES`, default 1 MiB). Past the cap, parsing stops but the user's stream continues normally.

#### http/https monkey-patch

`http.request(options, callback)` and `https.request(options, callback)` are replaced with wrappers that:

1. Compute the effective URL from `options` (or first arg if it's a URL/string).
2. Match against rewrite rules. If rewritten, edit `options.hostname`, `options.port`, `options.headers`, `options.path` before delegating.
3. Delegate to the saved original `request`.
4. Wrap the returned `ClientRequest` to observe `'response'`, `'error'`, `'socket'` events for timing.
5. Wrap the `IncomingMessage` to observe `'data'`, `'end'` events; parse a clone of the stream up to the parse-byte cap.

`http.get` and `https.get` are thin wrappers around `request` in Node; we patch them only to ensure the captured reference inside Node's standard library is also ours.

### 1.4 Rewrite rule model

Rules live in the interceptor bundle (static for v1; dynamic config in v2 — see Open Questions). Each rule:

| Field | Type | Description |
|---|---|---|
| `id` | string | Stable identifier (`anthropic-messages`, `openai-chat`, etc.); becomes a span attribute |
| `match.host` | string \| regex | Hostname match (exact or pattern) |
| `match.path_prefix` | string (optional) | Path-prefix filter; omitted means any path |
| `match.method` | string (optional) | HTTP method filter |
| `rewrite.origin` | string (template) | Replacement origin; supports `${ASTRO_AI_PROXY_URL}` |
| `rewrite.path_template` | string (optional) | Replacement path; supports `${path}` (original), `${upstream}` (rule id) |
| `rewrite.headers_add` | object (template) | Headers to add; values may reference env vars |
| `rewrite.preserve_host` | bool | If true, original Host header is preserved as `x-astro-original-host` |
| `extract` | string | Body extraction strategy. v1 values: `anthropic_messages` \| `openai_chat` \| `openai_responses` \| `openai_embeddings` \| `bedrock_invoke` \| `none`. See §1.6 for per-strategy parsing behavior. |
| `gen_ai_system` | string | Value for the `gen_ai.system` span attribute (`anthropic`, `openai`, etc.) |

A rule matches if all `match.*` predicates pass. The first matching rule wins. Rules with no `rewrite.*` apply only observation. If `ASTRO_AI_PROXY_URL` is unset, the `rewrite.origin` substitution fails, the rule's rewrite portion is skipped, and the request is observed only — never broken.

Rule set for v1:

| ID | Match | Rewrite target | Extraction |
|---|---|---|---|
| `anthropic-messages` | host = `api.anthropic.com`, path prefix `/v1/messages` | `${ASTRO_AI_PROXY_URL}/anthropic${path}` | `anthropic_messages` |
| `anthropic-other` | host = `api.anthropic.com` | `${ASTRO_AI_PROXY_URL}/anthropic${path}` | `none` |
| `openai-chat` | host = `api.openai.com`, path prefix `/v1/chat/completions` | `${ASTRO_AI_PROXY_URL}/openai${path}` | `openai_chat` |
| `openai-responses` | host = `api.openai.com`, path prefix `/v1/responses` | `${ASTRO_AI_PROXY_URL}/openai${path}` | `openai_responses` |
| `openai-embeddings` | host = `api.openai.com`, path prefix `/v1/embeddings` | `${ASTRO_AI_PROXY_URL}/openai${path}` | `openai_embeddings` |
| `openai-other` | host = `api.openai.com` | `${ASTRO_AI_PROXY_URL}/openai${path}` | `none` |
| `bedrock-invoke` | host matches `^bedrock-runtime\.[^.]+\.amazonaws\.com$`, path matches `^/model/[^/]+/invoke(-with-response-stream)?$` | `${ASTRO_AI_PROXY_URL}/bedrock${path}` (preserve_host=true for SigV4) | `bedrock_invoke` |

**No catch-all rule.** Traffic that does not match any rule above flows through the AstroDispatcher unmodified and **emits no span**. Network-level visibility into non-LLM egress is Beyla's job at the eBPF layer (host, port, byte counts, timing, status code via TCP teardown). Adding interceptor spans for every fetch call would duplicate Beyla's coverage while inflating cardinality and cost — especially for AWS-heavy agents that talk to S3, DynamoDB, SQS, etc. The interceptor's scope is intentionally narrow: route + observe LLM traffic, leave the rest to Beyla.

If we later need application-level visibility on non-LLM hosts (e.g., for protocol-aware spans on a customer's own backend API), it can be added as an opt-in via the future `agent.instrumentation.rules` field — explicit, per-host, not a fleet-wide default.

Bedrock host pattern is tightened to `bedrock-runtime.{region}.amazonaws.com` only — the previous draft's `.amazonaws.com` suffix would have matched every AWS service. Bedrock invoke paths are well-defined: `/model/<model-id>/invoke` for synchronous and `/model/<model-id>/invoke-with-response-stream` for SSE; the regex covers both. SigV4-signed requests can't have their host header trivially rewritten without re-signing, so `preserve_host=true` instructs the AI proxy to forward the SigV4 signature verbatim with the original host. If the AI proxy doesn't support SigV4 forwarding yet, the rule degrades to observation-only (no rewrite); the LLM call still goes direct.

Added headers on all rewritten requests:

| Header | Value | Purpose |
|---|---|---|
| `x-astro-upstream` | rule ID | Tells the AI proxy what upstream to forward to |
| `x-astro-agent-id` | `${ASTRO_AGENT_ID}` | Tenant attribution at the proxy (deployment ID — the codebase's canonical name for this value is `ASTRO_AGENT_ID`) |
| `x-astro-agent-name` | `${ASTRO_AGENT_NAME}` | Tenant attribution at the proxy |
| `x-astro-interceptor-version` | bundle version | Debugging |
| `traceparent` / `tracestate` | OTel context | Trace continuity across the proxy |

The original `Authorization` header is passed through unchanged. The proxy may strip and replace it with platform-managed credentials — that's the proxy's policy, not ours.

### 1.5 Response tap

For each request the interceptor handles, the response body is teed into a parser corresponding to the matched rule's `extract` strategy. The tap is byte-bounded and time-bounded:

- **Max bytes parsed**: 1 MiB (configurable). Past this, the parser stops; the user's stream is unaffected.
- **Max time**: 30 s after `onResponseStarted`. Past this, the parser stops.
- **Backpressure**: the tap is pull-mode against an internal buffer; if the parser falls behind, buffered chunks are discarded oldest-first and the span gets an attribute `astro.tap.truncated=true`.

The tap never touches the user's stream or buffers it on the user's path. A parse failure logs once at `[astropods-interceptor]` level and the rule's extraction degrades to a no-op for that request.

### 1.6 Token / usage extraction

#### Anthropic (rule: `anthropic_messages`)

Non-streaming response: response body is a single JSON object with `usage: { input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens }`. Extracted once at `onComplete` from the parsed body.

Streaming response (SSE):
- `message_start` event carries the final `usage.input_tokens` and a small initial `usage.output_tokens` (typically 1–2 for the start of the message).
- `message_delta` events carry `usage.output_tokens` as a **cumulative running total** of output tokens generated so far — *not* a per-event increment, despite the event name. This is per Anthropic's documented contract: ["The token counts shown in the `usage` field of the `message_delta` event are cumulative."](https://docs.anthropic.com/en/api/messages-streaming) One or more `message_delta` events may be emitted; the interceptor records `output_tokens` from each as it arrives and uses the latest value when finalizing the span.
- `message_stop` is the stream terminator; the interceptor finalizes the span here using `input_tokens` from `message_start` and the most recent `output_tokens` from `message_delta`.
- Other events (`ping`, `content_block_*`) are skipped.

Detection: Content-Type starts with `text/event-stream` → stream parser; else → JSON parser.

#### OpenAI Chat Completions (rule: `openai_chat`)

Non-streaming: body has `usage: { prompt_tokens, completion_tokens, total_tokens }`.

Streaming: only emits usage if the client passed `stream_options: { include_usage: true }`. We do not modify the request to force this — it would change behavior visible to user code. If usage is not emitted, the span carries `gen_ai.usage.available=false` and no token attributes. Document as a known gap.

#### OpenAI Responses API (rule: `openai_responses`)

The Responses API (`/v1/responses`) is a distinct surface from Chat Completions with different field names and SSE event types — extraction is a separate strategy.

Non-streaming: body has `usage: { input_tokens, output_tokens, total_tokens }` at the top level. Note the field names differ from Chat Completions (`input_tokens`/`output_tokens` here vs `prompt_tokens`/`completion_tokens` in Chat). The interceptor normalizes both into the `gen_ai.usage.input_tokens` / `gen_ai.usage.output_tokens` OTel span attributes.

Streaming (SSE):
- Event types are `response.created`, `response.in_progress`, `response.output_item.added`, `response.output_text.delta`, `response.completed`, `response.failed`, and others.
- Usage is carried by the `response.completed` event in `response.usage` with the same field names as the non-streaming case.
- The interceptor parses the `response.completed` and `response.failed` events only; all other events are skipped without parsing.
- Unlike Chat Completions, the Responses streaming API always emits usage in the terminal event — there is no `stream_options.include_usage` knob to worry about.

#### OpenAI Embeddings (rule: `openai_embeddings`)

Non-streaming only (no SSE). Body has `usage: { prompt_tokens, total_tokens }`. Extract both; map `prompt_tokens` to `gen_ai.usage.input_tokens`.

#### Bedrock (rule: `bedrock_invoke`)

Bedrock has per-model response shapes. v1 supports Claude-on-Bedrock and Llama-on-Bedrock; other models extract no usage and set `gen_ai.usage.available=false`.

### 1.7 Configuration

The interceptor reads only environment variables — never a file, never an HTTP call. See "Read-once semantics" below the table for read-time guarantees.

| Variable | Default | Set by | Purpose |
|---|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | spec_resolver (already on agent role) | OTel collector URL |
| `ASTRO_AI_PROXY_URL` | unset → no rewrite | spec_resolver (**new**) | Base URL of the AI proxy |
| `ASTRO_AGENT_ID` | — | spec_resolver (already on agent role; the deployment ID) | Tenant identifier — see note below |
| `ASTRO_AGENT_NAME` | — | spec_resolver (already on agent role) | Agent name |
| `ASTROPODS_INTERCEPTOR_DISABLED` | `0` | user override (debug) | Skip all patching |
| `ASTROPODS_INTERCEPTOR_LOG_LEVEL` | `warn` | platform default | `error` \| `warn` \| `info` \| `debug` |
| `ASTROPODS_INTERCEPTOR_MAX_BODY_PARSE_BYTES` | `1048576` | platform default | Cap on parsed body size |
| `ASTROPODS_INTERCEPTOR_TAP_TIMEOUT_MS` | `30000` | platform default | Cap on tap duration |

**Note on `ASTRO_AGENT_ID`.** The codebase already surfaces the deployment ID to every per-deployment container under the env var name `ASTRO_AGENT_ID` (see `apps/astro-server/internal/deployment/spec_resolver.go:82` and `resolve.go:164`). Despite the semantic name being "deployment ID," the canonical env var is `ASTRO_AGENT_ID`. The interceptor reads this existing variable rather than introducing a redundant `ASTRO_DEPLOYMENT_ID` — no `resolve.go` change is required for tenant attribution.

**Only one new agent-role env var is strictly required:** `ASTRO_AI_PROXY_URL`, set by `spec_resolver.go` (§3.2). The `ASTROPODS_INTERCEPTOR_*` variables can be added later as needed; sensible defaults are baked into the interceptor.

#### Read-once semantics

All variables in the table above are read **once at bootstrap** and cached. The interceptor does not re-read `process.env` per request. Two consequences:

- Changing any of these env vars on a running pod requires a pod restart to take effect — including `ASTRO_AI_PROXY_URL`, the cluster-wide bypass switch (§4.2). For v1 this is the accepted operational model; live-reload is in scope for v2 only.
- User code mutating `process.env` after interceptor bootstrap does not affect interceptor behavior.

`ASTROPODS_INTERCEPTOR_DISABLED` is intentionally readable from the user environment as an escape hatch. Setting it does not remove the entrypoint wrapper or the file from `/opt/astro/`, just disables runtime patching.

### 1.8 Failure containment

The interceptor must never crash the agent. Hierarchy of failure:

1. **Module file missing or zero-length**: the entrypoint wrapper's pre-flight check (§2.3 step 3) detects this and skips `NODE_OPTIONS` injection entirely. The agent starts without instrumentation; a single warning is logged. No crash.
2. **Module file syntactically broken (non-zero length, invalid JS)**: Node accepts `--require` (the file exists) and aborts when it tries to parse. The mirror smoke test (§9.2) catches this before any image is published, so this failure mode should not reach production. If it does, the entrypoint wrapper's pre-flight check does not catch it (size check is not a syntax check); the agent crashes on startup. We accept this gap because the alternative — parsing JS in the wrapper — is expensive and brittle.
3. **Bootstrap throws**: top-level try/catch logs and proceeds. User code runs without instrumentation.
4. **Patch installation throws** (e.g., undici's API changed in a Node version we haven't tested): the failing patch is skipped; remaining patches install. Logged once.
5. **Rule match throws**: the request passes through unmodified. Span is emitted with `astro.interceptor.error="match"`.
6. **Rewrite throws**: the request passes through to its original destination. Span gets `astro.interceptor.error="rewrite"`.
7. **Handler wrapping throws**: the original handler is used directly. No span.
8. **Tap parser throws**: parsing stops for that request; span is emitted without usage attributes.
9. **OTel export fails**: spans are dropped; no retry, no buffering (collector is responsible for ingestion reliability).

No failure mode in the interceptor produces a user-visible behavior change in the request itself.

## 2. Delivery: registry-mirror base image substitution

The interceptor is delivered exclusively through patched Node base images served by `astro-registry`. There is no per-build decoration step, no post-build image transformation, no fallback path. Agents whose Dockerfile uses a mirrored base are instrumented automatically; agents using anything else are not. This section pins down what the mirror contains, how the patched bases are built, how pulls route through our registry, and what happens at the boundary.

### 2.1 The base image mirror

A finite set of patched Node base images, maintained by Astro and served from `astro-registry` under the same canonical paths as upstream Docker Hub. v1 mirrors:

| Upstream tag pattern | Notes |
|---|---|
| `library/node:18`, `library/node:18-alpine`, `library/node:18-slim`, `library/node:18-bookworm` | Minimum supported Node version |
| `library/node:20`, `library/node:20-alpine`, `library/node:20-slim`, `library/node:20-bookworm` | Primary tested baseline |
| `library/node:22`, `library/node:22-alpine`, `library/node:22-slim`, `library/node:22-bookworm` | |
| `library/node:24`, `library/node:24-alpine`, `library/node:24-slim`, `library/node:24-bookworm` | |

Each tag is built for both `linux/amd64` and `linux/arm64`, published under the same manifest list. Specific subversion tags (e.g. `node:20.10.5`) are *not* mirrored at v1; users who pin to a specific patch version get the upstream image and are not instrumented.

Coverage is bounded by this set. Expanding coverage means adding tags to the mirror, not adding code.

### 2.2 How a patched base is built

For each mirrored tag, a CI pipeline at `packages/node-base-mirror/` produces a patched image by adding a single layer on top of upstream:

```
FROM <upstream-tag>
COPY --from=astropods/node-interceptor:<version> /interceptor/interceptor.cjs /opt/astro/interceptor.cjs
COPY --from=astropods/node-interceptor:<version> /interceptor/entrypoint     /opt/astro/entrypoint
LABEL org.astropods.interceptor.version=<version>
LABEL org.astropods.base-mirror.upstream-digest=<upstream-image-digest>
ENTRYPOINT ["/opt/astro/entrypoint", <upstream ENTRYPOINT...>]
CMD [<upstream CMD...>]
```

The upstream image's original `ENTRYPOINT` and `CMD` are read at build time and templated into the wrapper invocation element-by-element so they survive shell metacharacters. The patched image is pushed to `astro-registry` under the same path the upstream tag would resolve to from Docker Hub (e.g. `astro-registry/library/node:20`). Build is done with BuildKit; multi-arch is handled by the inner `COPY --from=` resolving per-arch under the manifest list.

The build pipeline is small enough to live as a templated Dockerfile + a thin Go driver in `packages/node-base-mirror/` that enumerates the tag set, queries upstream metadata for each, generates the per-tag Dockerfile, invokes BuildKit, and pushes the result. Future runtimes contribute their own driver under the same pattern (see §11).

### 2.3 The entrypoint wrapper

A single wrapper variant ships per architecture: a static Go binary (`entrypoint`). Using one binary for all images — including those with `/bin/sh` — is preferred over a shell-script variant because:

- No dependency on which shell (`sh`, `bash`, `dash`, `ash`, `busybox sh`) the image happens to provide.
- No POSIX-vs-bashism portability traps when parsing argv.
- Unit-testable in Go; smoke-tested against fixture argv shapes (`node ...`, `sh -c "node ..."`, `tini -- node ...`, `bun ...`, etc.).
- Works on distroless / scratch images that have no shell, with no code-path divergence.

The binary is built `CGO_ENABLED=0` and stripped (~2 MB per arch). On exec it:

1. Reads its own argv (everything after the binary itself = the original ENTRYPOINT + CMD).
2. Walks past known PID-1 wrappers (`tini`, `dumb-init`, `gosu`, `s6-svscan`, `catatonit`) and their flag arguments to find the real target binary.
3. **Pre-flight check**: `stat(/opt/astro/interceptor.cjs)`. If the file is missing, zero-length, or unreadable, skip injection entirely and log a single warning line to stderr (`[astropods-interceptor] interceptor file missing or empty at /opt/astro/interceptor.cjs; running uninstrumented`). The agent continues to run. This catches the obvious corruption modes — missing file, partial download, zero-length write — without giving Node a broken module to require. It does not catch a syntactically broken bundle, which is the mirror smoke test's job (§9.2).
4. If the target's basename is exactly `node` **or `nodejs`**, prepends `--require=/opt/astro/interceptor.cjs` to `NODE_OPTIONS` (preserving any existing value). Both names are accepted because Debian/Ubuntu historically ship the binary as `nodejs` (the `node` name collided with the `ax25-node` package); many container images still invoke it that way.
5. If the target is `sh` / `bash` / `dash` / `ash` followed by `-c <command>`, parse `<command>` (see "shell command scanning" below) and inject `NODE_OPTIONS` only if the parser identifies a `node` or `nodejs` invocation.
6. For any other target (bun, deno, python, ruby, custom script, native binary): no env change. This is **runtime confirmation** — even though the base image is a Node mirror, the user could still invoke a non-Node binary as their entrypoint, and the wrapper handles that gracefully by skipping injection.
7. `syscall.Exec`s the original argv with the (possibly modified) environment.

#### Shell command scanning

`sh -c "<command>"` argv parsing must not false-positive on substrings (`download_node_modules`, `NODE_ENV=production`) or miss real invocations (`/usr/local/bin/node server.js`, `cd /app && node server.js`). The algorithm:

1. Split the command string on shell statement separators: `;`, `&&`, `||`, `|`, `&`. Each resulting segment is a statement.
2. For each segment, tokenize on whitespace (respecting quotes — the wrapper uses a minimal POSIX-quoting tokenizer; full shell semantics are out of scope).
3. From the segment's tokens, skip leading tokens that match the env-var-assignment pattern (`^[A-Z_][A-Z0-9_]*=`). These are inline env vars like `NODE_ENV=production`, not commands.
4. The first non-assignment token is the command word. Compare its **basename** (last path segment) for case-sensitive equality with `node` or `nodejs`.
5. If any segment's command word is `node` or `nodejs`, inject `NODE_OPTIONS`. If none match, do not inject.

Examples of behavior:

| Command string | Decision | Why |
|---|---|---|
| `node server.js` | inject | basename `node` matches |
| `nodejs server.js` | inject | basename `nodejs` matches (Debian/Ubuntu binary name) |
| `/usr/local/bin/node server.js` | inject | basename `node` matches |
| `/usr/bin/nodejs server.js` | inject | basename `nodejs` matches |
| `NODE_ENV=production node server.js` | inject | `NODE_ENV=` skipped as assignment; next token `node` matches |
| `NODE_ENV=production python app.py` | skip | command word is `python` |
| `download_node_modules && python server.py` | skip | first segment's command word is `download_node_modules`, second's is `python` |
| `echo running; cd /app && node server.js` | inject | third segment's command word is `node` |
| `bun server.js` | skip | command word is `bun` |
| `bash -c "node server.js"` | inject (recursive) | outer command word is `bash` with `-c`; the inner command is parsed recursively by the same rules |
| `exec node server.js` | inject | `exec` is a shell builtin; treated as a transparent prefix, similar to env assignments |

The recursive `bash -c "..."` case is handled with a hard depth limit (3) to bound parser runtime against pathological input. The `exec` shell builtin is recognized and skipped as a one-token prefix; other builtins (`eval`, `source`, `.`) are not — agent images that wrap their entrypoint through `eval` are exotic enough that we accept "no injection" as the safe outcome there.

The algorithm is implemented in Go in the wrapper binary and is unit-tested against the full table above plus a fuzz corpus.

The wrapper holds PID 1 only momentarily — `syscall.Exec` replaces the wrapper process with the original entrypoint, so signal handling, zombie reaping, etc. remain the user's image's responsibility (typically delegated to `tini` if they use it).

### 2.4 Refresh lifecycle

The base image mirror is rebuilt by a scheduled CI pipeline:

- **Daily**: re-resolve every mirrored tag against upstream Docker Hub. If the upstream digest changed (typically a Node security patch), rebuild and republish.
- **On interceptor version bump**: rebuild all mirrored tags with the new interceptor version. Published as a new image manifest; old versions remain available until they age out.
- **Lag budget**: target < 24 h from upstream patch publication to mirror availability. Users pulling within the lag window get the previous patched version; already-built agent images are unaffected.

The pipeline uses BuildKit. Image signing is done with our own keys (signature transparency for our patches; we do not preserve upstream signatures, which would be impossible).

### 2.5 Routing pulls through `astro-registry`

The mechanism is a **registry mirror**: `astro-registry` serves patched versions of recognized public Node tags under their canonical Docker Hub paths, and pulls of those tags resolve there. The user's Dockerfile is never modified — neither on disk nor in memory. `FROM node:20` stays `FROM node:20`; only the *registry endpoint that resolves it* changes.

There are two pull contexts in our build infrastructure, and each routes through the mirror in the natural way for that context:

- **Cloud builder**: BuildKit is configured with `astro-registry` as a registry mirror for `docker.io`. Pull resolution is fully transparent — the builder sees `FROM node:20`, resolves through the configured mirror, and gets our patched image. No CLI-side logic, no manifest substitution, no Dockerfile awareness. This is the canonical implementation of the mirror approach.
- **CLI local build**: the CLI today calls `cli.ImagePull(ctx, "node:20", ...)` and `cli.ImageBuild(ctx, ...)` against the user's local Docker daemon, which by default pulls from Docker Hub (verified in `apps/astro-cli/cmd/build_runner.go:216`). Configuring the user's daemon to use our registry as a mirror would require touching their machine — not zero-touch. Instead, the CLI achieves the equivalent effect entirely in-process:
  1. Parses the Dockerfile to identify base image references (existing `parseDockerfileBaseImages` function).
  2. For each reference matching a mirrored tag pattern (e.g. `node:20`, `library/node:20`, `docker.io/library/node:20`), pulls the **patched version from `astro-registry`** via `cli.ImagePull(ctx, "astro-registry.com/library/node:20", authOpts)`.
  3. Tags the pulled image locally under the **original reference** the user wrote (`node:20`) via `cli.ImageTag`.
  4. Builds with the existing Dockerfile unchanged. When the daemon resolves `FROM node:20`, it finds the patched image already in local cache and uses it.
  
  The user's Dockerfile is not read for rewriting purposes, not parsed for substitution, not modified anywhere. The CLI's only role is **redirecting which registry the pull comes from**, achieving the same end state as a daemon-level mirror without daemon-level configuration.

#### Multi-stage Dockerfiles

`parseDockerfileBaseImages` (`apps/astro-cli/cmd/build_runner.go:132`) returns the `FROM` reference from every stage, filtering only `scratch` and stage aliases (`AS builder`). The CLI redirects **every** recognized reference through `astro-registry`, regardless of which stage uses it. This is intentional and chosen for simplicity:

- Pulling a patched `node:20` for a builder-only stage costs one cached layer pull. Harmless.
- Whether the final agent image is instrumented is determined by the runtime stage's `FROM`, not by what the builder used. If the runtime stage uses a mirrored base, the agent is instrumented; if not, it isn't.
- Identifying "the runtime stage" precisely would require honoring the optional `build.target` field in `astropods.yml` and tracing the stage dependency graph. Worth the complexity only if pulling extra mirror layers becomes a real cost; for v1, pulling all matching references is correct and trivial.

### 2.6 Behavior at the coverage boundary

Agents whose Dockerfile doesn't use a mirrored base get **no instrumentation**. This is by design; we deliberately do not add a second mechanism to catch the long tail. Specifically:

| Case | What happens |
|---|---|
| `FROM ubuntu:22.04` + manual Node install | CLI's base-image matching doesn't recognize the reference. Pull resolves through Docker Hub as normal. Built image has no `/opt/astro` directory. Agent runs uninstrumented; emits no spans. |
| `FROM gcr.io/distroless/nodejs20` | Not Docker Hub, not in mirror set. Same as above. |
| `FROM node:20.10.5` (specific subversion) | Subversion tags not mirrored at v1. Pull resolves through Docker Hub. Uninstrumented. |
| `FROM mycompany.com/internal/node:20` (custom corporate base) | Not Docker Hub. Same as above. |
| Pre-built external image referenced by `agent.image:` | Already built outside Astro's pipeline. No pull-time redirection possible. Uninstrumented. |
| User opts out via `agent.instrumentation: false` | CLI skips pull redirection entirely. Pulls of `node:20` resolve through Docker Hub. Uninstrumented. (See §3.1.) |

The user-visible signal is "no spans appear in observability." Astro-side, the absence of the `org.astropods.interceptor.version` label on the deployed agent image is detectable at deploy time and can drive a UI warning, a deploy-log line, or both — flagged in Open Questions for product to decide on the exact treatment. The deploy itself does not fail; uninstrumented agents are first-class.

To expand coverage, add tags to the mirror set (§2.1). If a class of users routinely hits the boundary, the answer is to grow the mirror — not to introduce per-image patching.

### 2.7 Per-build-path integration

| Path | Mechanism | Wire-up |
|---|---|---|
| Local CLI (`astro deploy` / `astro push`) | CLI pulls recognized base images from `astro-registry` and tags them locally under the user's original reference (§2.5); the user's Docker daemon then resolves `FROM node:20` from the local cache during build. Dockerfile is unchanged. Bases outside the mirror set are not redirected; build proceeds upstream and the agent ships uninstrumented. | `apps/astro-cli/cmd/build_runner.go`'s `prePullBaseImages` is extended to pull-and-tag mirrored references. |
| Cloud builder (GitHub integration) | BuildKit is configured to use `astro-registry` as a Docker Hub mirror. Pulls of `docker.io/library/node:*` transparently resolve to our patched mirror. Pulls of unmirrored references resolve to upstream as normal. Dockerfile is unchanged. | BuildKit registry-mirror configuration. No per-build code. |
| Pre-built external image (`agent.image:`) | Build already happened externally. Image is pulled into internal ECR via existing `image_resolver` logic and deployed as-is. Uninstrumented. | No new wiring; existing path applies. |

### 2.8 Versioning

- `astropods/node-interceptor` is a versioned image tag (`v1`, `v1.2.3`, …). Never `latest`. Used as the `COPY --from=` source by the mirror build pipeline.
- The interceptor version is a constant in the `packages/node-base-mirror/` driver. Bumping it triggers a rebuild of all mirrored tags.
- Patched base images carry `org.astropods.interceptor.version` (current interceptor version) and `org.astropods.base-mirror.upstream-digest` (upstream version this is derived from). The labels propagate to downstream agent images automatically — agent images built `FROM` a patched base inherit the labels, which lets operators identify which interceptor version any given pod is running.
- Existing built images are not invalidated by an interceptor version bump; they continue to run their pinned version until their owning agent is redeployed.

## 3. Deployment Integration

### 3.1 Spec changes

One optional field added to `agent` in `astropods.yml`: **`agent.instrumentation`**. The field is polymorphic — it accepts a bare boolean for the common case or an object for future fine-grained controls.

#### Accepted forms

```yaml
# Default — field absent. Equivalent to `instrumentation: true`.
agent:
  build: { ... }

# Bare-boolean form.
agent:
  build: { ... }
  instrumentation: true       # explicitly on (same as omitting)
  # or
  instrumentation: false      # fully opt out

# Object form. Reserved for future knobs; `enabled` is the only v1 field.
agent:
  build: { ... }
  instrumentation:
    enabled: false
```

#### Schema

`oneOf: [ boolean, object ]`. The object form is:

| Field | Type | Default | Purpose |
|---|---|---|---|
| `enabled` | bool | `true` | Master switch for the auto-instrumentation feature on this agent |

Forward-compatibility: future knobs land as additional object fields (e.g., `captureRequestHeaders`, `rewriteLLMTraffic`, `rules`). The bare-boolean form is always equivalent to `{ enabled: <bool> }` — adding fields later doesn't break old specs.

#### Parser behavior

In Go (`packages/astro-spec/spec.go`), a custom `Instrumentation` type with `UnmarshalJSON` / `UnmarshalYAML`:
- JSON/YAML `true` → `Instrumentation{Enabled: true}`
- JSON/YAML `false` → `Instrumentation{Enabled: false}`
- JSON/YAML object → parsed as the struct
- Absent → zero value, treated as `Enabled: true` (the spec default sentinel is "absence means on")

Equivalent handling on the TypeScript side wherever the spec is consumed in JS.

#### Semantics

When `instrumentation.enabled == false`:
- The CLI does not redirect base-image pulls through the patched mirror. The build proceeds entirely against upstream sources, and the resulting image is uninstrumented.
- Interceptor-specific env vars (`ASTRO_AI_PROXY_URL`, `ASTROPODS_INTERCEPTOR_*`) are not injected.
- The existing collector / OTel pipeline is **unaffected**. If the user wires up `setupObservability()` manually or runs their own OTel SDK, that still works — the collector sidecar is controlled by the server-managed `DeploymentObservability` field, not by this flag.

This is the only user-facing opt-out. There is no image LABEL mechanism. `ASTROPODS_INTERCEPTOR_DISABLED=1` (§6.2) can additionally disable the interceptor at *runtime* in an already-built image, but the spec field is the deploy-time control.

### 3.2 Env var injection (spec_resolver)

`apps/astro-server/internal/deployment/spec_resolver.go` already emits `OTEL_EXPORTER_OTLP_ENDPOINT`, `ASTRO_AGENT_ID` (the deployment ID — see §1.7), and `ASTRO_AGENT_NAME` into the agent's resolved environment. Two new variables:

| Variable | Value | Notes |
|---|---|---|
| `ASTRO_AI_PROXY_URL` | per-environment URL of the AI proxy | Unset in environments where the proxy isn't deployed; interceptor degrades to observe-only |
| `ASTROPODS_INTERCEPTOR_LOG_LEVEL` | from environment config | Allows raising the log level cluster-wide for debugging |

No changes to `internal/k8s/deployment.go` are required — the env vars flow through the existing `deployment_build_env` projection.

### 3.3 Telemetry pipeline (no changes)

The interceptor emits OTel spans to `OTEL_EXPORTER_OTLP_ENDPOINT`, which is already the per-deployment collector. The `astro` processor in `packages/astro-collector` already enriches spans with deployment / agent attributes. Span attributes follow the existing `gen_ai.*` and `astro.*` semantic conventions used by the Mastra-emitted spans, so the downstream Galileo / Langfuse pipelines need no changes.

One backend implication: spans emitted by the interceptor and spans emitted by the legacy `setupObservability` adapter could coexist during migration. The interceptor sets `astro.span.source="interceptor"`; the adapter sets nothing. Backend tooling can filter or deduplicate on this attribute.

## 4. AI Proxy Interface

The AI proxy itself is a separate spec. This section pins only the wire-level contract the interceptor depends on.

### 4.1 Request shape

The interceptor rewrites outbound requests as:

```
<METHOD> ${ASTRO_AI_PROXY_URL}/<upstream>/<original-path-and-query>
Host: <as derived from ASTRO_AI_PROXY_URL>
Authorization: <unchanged from original>
x-astro-upstream: <rule id>
x-astro-agent-id: <id>   # value of ASTRO_AGENT_ID env var (= deployment ID)
x-astro-agent-name: <name>
x-astro-interceptor-version: <version>
x-astro-original-host: <original host, only if rule had preserve_host=true>
traceparent / tracestate: <OTel context>
<all other original headers passed through>

<original body, unmodified>
```

The proxy is responsible for:
- Validating the `x-astro-agent-id` header against its own tenant context.
- Forwarding to the upstream — host derived from the rule ID, path is the post-prefix portion.
- Returning the upstream response verbatim (or with body modifications if the proxy chooses to).

### 4.2 Failure modes the interceptor handles

- **Proxy unreachable / TCP refused**: the request fails. The user's SDK gets a connection error.
- **Proxy returns 5xx**: passed back to the user code as the response. The interceptor sets `astro.proxy.error=true` on the span.
- **Proxy returns 4xx that the user code can't interpret** (e.g., proxy auth failure): the interceptor sets `astro.proxy.error=true`; user code sees a 4xx from "Anthropic" that they didn't expect. Cost of routing through the proxy.

#### No per-request silent fallback

The interceptor never silently retries against the original upstream when the proxy fails. Per-request fallback is rejected because:

1. **Policy bypass.** Quotas, budget caps, key rotation, prompt redaction, and content policy all live at the proxy. Silent fallback creates windows where these are bypassed without operator awareness.
2. **Hides problems.** A degraded proxy returning intermittent errors would be invisible if masked by fallback.
3. **Behavioral nondeterminism.** Identical requests would take different paths depending on transient health — hard to debug, hard to reason about.
4. **Trust model.** "All LLM traffic flows through the proxy" is a clean contract that callers can rely on. "Most LLM traffic except when…" is leaky.

The accepted risk: a proxy outage degrades the LLM hot path for every Node agent. This is treated as an operational issue — if our proxy is down, we have something to fix on our end regardless. Mitigation is operator-driven, not automatic.

#### Mitigation: cluster-wide bypass

When the proxy is degraded or down, an operator sets `ASTRO_AI_PROXY_URL` to empty (or unsets it) for the affected environment, then triggers a rolling restart of agent deployments. The interceptor reads this value once at bootstrap (§1.7); on the restarted pods, the rewrite step short-circuits and all LLM calls flow direct to upstream. Observation (spans for matched rules) continues normally — only routing is bypassed.

Two notes on this:
- The bypass is **environment-wide**, not per-request or per-deployment. It is a deliberate, operator-controlled switch, not an automatic fallback. The contract "either all LLM traffic in this env goes through the proxy, or none does" is preserved at any given moment.
- Activating the bypass requires the env var change to reach already-running agent pods. Kubernetes env vars sourced from ConfigMaps are read **once at pod start** — they do not update live. v1 accepts this and requires a rolling restart of agent deployments to apply the bypass. For most outages, restart latency (seconds to a minute) is acceptable. If faster activation becomes a hard requirement, v2 can add a file-mounted config the interceptor re-reads per request, or a small control-plane fetch on startup with a long-poll for changes. Both are real work and explicitly out of scope for v1.

A per-request automatic circuit-breaker is a candidate for v2 if v1 operations show that human-driven bypass is too slow to react. v1 explicitly does not include it to keep the design simple and the proxy-traffic contract unambiguous.

## 5. Edge Cases

Concrete behaviors for the edge cases enumerated during design:

| Case | Behavior |
|---|---|
| User's `NODE_OPTIONS` already set (Dockerfile ENV) | Entrypoint wrapper appends ours: `NODE_OPTIONS="--require=/opt/astro/interceptor.cjs ${NODE_OPTIONS}"`. Our flag goes first so we load before transpilers (`ts-node/register`, `tsx`). |
| K8s pod spec sets `NODE_OPTIONS` via env var | K8s env overrides Dockerfile ENV. The wrapper still merges from the env at startup, so K8s additions are preserved. |
| `child_process.spawn('node', ...)` from user code | `NODE_OPTIONS` propagates to children; interceptor runs in each. |
| `child_process.fork(...)` / `worker_threads.Worker` | Same — Node propagates `NODE_OPTIONS` to forks. Interceptor's bootstrap is idempotent. |
| `child_process.spawn('curl', ...)` | Not Node, no interception. Beyla sees metadata. Document. |
| Process supervisors (pm2, nodemon, tsx watch) | Interceptor runs in both supervisor and children. The supervisor process makes no LLM calls, so spans emitted from it are zero-volume. No special-case needed. |
| User has their own OTel SDK (`@opentelemetry/sdk-node`) | Detected via `@opentelemetry/api`'s `trace.getTracerProvider()` returning a real provider. Interceptor uses the user's provider instead of installing its own. |
| User calls `setupObservability()` (legacy path) | Adapter detects interceptor presence via `globalThis[Symbol.for("astropods.interceptor")]` (§1.2 step 8) and skips its own setup, logging a deprecation notice. |
| User monkey-patches `globalThis.fetch` after our patch loads | Their wrapper sits above ours. Their fetch still uses undici under the hood, which still flows through `AstroDispatcher`. Defense-in-depth holds. |
| `axios` / `got` / `node-fetch@2` (use `http.request` directly) | Caught by the `http`/`https` monkey-patch. |
| `@grpc/grpc-js` | Out of scope. Spans not emitted for gRPC traffic. Document. |
| HTTP/2 via `http2.connect` | Out of scope. Most LLM providers don't expose HTTP/2 raw — fetch goes through HTTP/1.1 or HTTP/2 via undici. Native HTTP/2 not patched. |
| WebSocket (`ws`) | Out of scope. Document. |
| Streaming response: parser falls behind | Tap discards oldest buffered chunks; span sets `astro.tap.truncated=true`. User stream is unaffected. |
| Streaming response: SDK uses `stream_options: { include_usage: true }` (OpenAI) | Usage extracted. |
| Streaming response: SDK does *not* request usage (OpenAI) | Span emitted without usage attributes; `gen_ai.usage.available=false`. Known gap. |
| Read-only root filesystem (`readOnlyRootFilesystem: true`) | Interceptor writes nothing at runtime. `/opt/astro/` is baked in at build time. |
| `tini` / `dumb-init` as ENTRYPOINT (inside a mirrored Node base) | The patched ENTRYPOINT runs our wrapper first; the wrapper recognizes `tini` / `dumb-init` and walks past them to find the real target. Tini still receives PID 1 semantics through `exec`. |
| User signs their own image (Notation, Cosign) on top of `FROM node:20` | The patched base from our mirror is signed with our keys, not the upstream's. Users who must preserve upstream signature continuity set `agent.instrumentation: false` and accept uninstrumented agents. |
| User image has both `node` and `bun` binaries; ENTRYPOINT is `sh -c "..."` | If `FROM` is a mirrored Node base, the wrapper is present. At runtime the wrapper scans the `-c` command and injects `NODE_OPTIONS` only if `node` is the invoked binary. Bun invocation runs unmodified. |
| User runs `node migrate.js` as a one-off Job (not a long-running Deployment) | If the Job's image is a mirrored Node base, the interceptor still loads. Span emission must flush in `beforeExit`. |
| `process.env` mutated by user code at runtime | Interceptor reads env once at bootstrap; later mutations don't affect it. |
| User runs Node with `--zero-fill-buffers` / `--experimental-vm-modules` | Coexists with `--require` — both are honored in any order. |
| User image is multi-arch (amd64 + arm64) | Both arches are mirrored under the same manifest list; whichever arch the user pulls gets the patched version. |
| Same upstream Node version used by many agents | All agents pull the same patched base from the mirror; one cached layer is reused fleet-wide. |
| Interceptor version bump mid-flight | Mirror tags are rebuilt with the new version; already-built agent images continue to run their pinned version until redeployed. |
| User's image has `ENTRYPOINT []` (empty) and only `CMD` | The upstream Node base's CMD is preserved by our wrapper; if the user overrides CMD, the wrapper still resolves the new command correctly. |
| User's image has `ENTRYPOINT` as a string (shell form, not exec form) | Resolved as `/bin/sh -c "<entrypoint string>"` by Docker before reaching our wrapper. The wrapper detects `sh -c` and parses the command as described in §2.3. |
| Interceptor file missing or zero-length in patched mirror image | Entrypoint wrapper's pre-flight check (§2.3 step 3) skips `NODE_OPTIONS` injection and logs a warning. Agent runs uninstrumented. |
| Interceptor file present but syntactically broken (corrupted layer, etc.) | Mirror smoke test (§9.2) catches this before publish. If it slips past CI, Node aborts on require — wrapper's pre-flight size check does not detect syntax errors. Documented gap; we accept it because the mirror CI is the right place to catch this. |
| User pins to a specific Node patch version (`FROM node:20.10.5`) | Not in v1 mirror set. CLI's base-image matching doesn't find a match; pull resolves to Docker Hub. Agent runs uninstrumented. |
| User uses `gcr.io/distroless/nodejs20` or another non-Docker-Hub Node base | Not in the mirror set. Pull resolves to upstream registry. Agent runs uninstrumented. |
| Upstream Node releases a security patch (e.g., `node:20` digest changes) | Mirror refresh CI (§2.4) detects the digest change daily and rebuilds. Lag budget < 24 h. Already-built agent images are unaffected; new builds during the lag window get the previous patched version. |
| User's local `docker pull node:20` (outside the CLI) gets the upstream version, not ours | Expected. The CLI's redirection is in-process before calling the Docker daemon; it does not configure the daemon. Users running `docker` directly bypass our infrastructure and see upstream behavior. |
| User has a Docker daemon configured with `registry-mirrors` for `docker.io` | Their mirror config affects pulls the daemon initiates on its own. The CLI's redirection works at the SDK call level (pull through `astro-registry` directly and tag locally), so the build's `FROM node:20` resolves from local cache before the daemon's mirror kicks in. End result: instrumented image, same as the no-mirror case. |
| User's Dockerfile uses `ARG NODE_VERSION` then `FROM node:${NODE_VERSION}` | The CLI's base-image matching evaluates ARG defaults and any `--build-arg` overrides to resolve the concrete tag, then checks the mirror set. If the resolved tag is in the set, the CLI pulls the patched version and tags locally under the user's reference; otherwise pull resolves to upstream and the agent is uninstrumented. |
| Multi-stage Dockerfile with `FROM node:20 AS builder` and `FROM gcr.io/distroless/nodejs20` (runtime) | CLI redirects all matching `FROM` refs across all stages (§2.5 multi-stage subsection), so `node:20` gets pulled from the patched mirror for the builder. The runtime stage's `FROM` is what determines final-image instrumentation: distroless isn't in the mirror set, so the agent is uninstrumented. Patched-base layers in the builder stage don't reach the final image. |

## 6. Failure and Operational Model

### 6.1 Production failure modes

| Failure | User-visible impact | Astro-visible signal |
|---|---|---|
| Interceptor bootstrap throws | None (interceptor no-ops) | stderr log, missing spans for that pod |
| AI proxy down | LLM calls fail with connection error | Span with `astro.proxy.error=true` |
| Tap parser slow | None (tap truncates) | Span with `astro.tap.truncated=true` |
| OTel collector down | None | Spans dropped (no buffering in interceptor) |
| Base mirror unavailable / pull from `astro-registry/library/node:*` fails | Build can't proceed (the user's Dockerfile requires the base image to pull). User gets a clear error. | `astro-registry` health metrics; build failure rate by registry path. |
| Base mirror stale relative to upstream (after a Node CVE) | User pulling `node:20` gets the patched version of the previous upstream digest until refresh CI runs (< 24 h lag, §2.2.3). User does not see broken builds, just slightly out-of-date base. | Mirror-age metric exported by the refresh CI pipeline. |
| Mirror build pipeline fails for a tag (upstream pull error, BuildKit error) | New users pulling that tag get the previous patched version (or the upstream, if no prior patched version was published). | Mirror CI alert; refresh-age metric trips threshold. |
| Interceptor version skew (patched image runs against changed proxy contract) | Possibly broken rewriting | Version label on image identifies pinning |

### 6.2 Kill switches

| Scope | Mechanism | What happens |
|---|---|---|
| **Per-agent, at deploy time** | `agent.instrumentation: false` in `astropods.yml` | CLI skips the pull-redirection step. `FROM node:20` resolves through upstream Docker Hub. Built image is uninstrumented. |
| **Per-deployment, at runtime** | Set `ASTROPODS_INTERCEPTOR_DISABLED=1` as a deployment variable | The already-patched image still runs the wrapper, but the interceptor module returns early on bootstrap. No patches installed. Lets us disable the interceptor without rebuilding the image. |
| **Cluster-wide proxy bypass** | Unset `ASTRO_AI_PROXY_URL` for an environment (or set to empty) | Interceptor still observes (spans emitted), but the rewrite step short-circuits and all LLM traffic flows direct to upstream. **This is the documented mitigation for AI-proxy outages — see §4.2.** |
| **Cluster-wide interceptor off** | Pin `astropods/node-interceptor` to a no-op build and re-run the mirror pipeline | Already-built agent images continue running their pinned interceptor version. New mirror builds incorporate the no-op; users who pull the mirror after that get an uninstrumented patched base. Use for a rapid rollback of the interceptor itself. |

### 6.3 Metrics emitted by the interceptor

Counter / gauge attributes published as OTel metrics (alongside spans):

- `astropods.interceptor.bootstrap_success` (boolean, set once per process)
- `astropods.interceptor.requests_total` (counter, by rule)
- `astropods.interceptor.rewrites_total` (counter, by rule)
- `astropods.interceptor.tap_truncations_total` (counter, by rule)
- `astropods.interceptor.parse_errors_total` (counter, by rule + reason)
- `astropods.interceptor.patch_failures_total` (counter, by layer: `undici_dispatcher` / `undici_named_export` / `http_request` / etc.)

## 7. Migration

### 7.1 Sunset of the Mastra observability adapter

`modules/adapters/packages/mastra/src/observability.ts:setupObservability` becomes redundant once the interceptor ships. Migration:

1. **Release N**: ship the interceptor. `setupObservability` detects the interceptor (by checking `globalThis[Symbol.for("astropods.interceptor")]`) and no-ops with a `logger.info` line. Existing user code that calls `setupObservability` continues to work.
2. **Release N+1**: `setupObservability` logs a deprecation warning.
3. **Release N+2 (≥3 months later)**: remove `setupObservability` and the `@astropods/adapter-mastra` observability surface entirely.

Until step 3, the adapter remains importable for users who haven't redeployed — their old images don't get the interceptor and they still need the adapter to emit spans.

### 7.2 Existing deployments

No automatic rebuild of existing user images. The interceptor takes effect on the next deploy after the feature ships, when the user's build pulls a patched mirror base for the first time. We do not force a fleet-wide redeploy.

### 7.3 Backend dedup

The Astro processor in `packages/astro-collector` may receive both interceptor-emitted and adapter-emitted spans for the same fetch call during the migration window. The processor adds a deduplication step: when two spans share the same `traceparent` and the same `http.url` + `http.method`, drop the one without `astro.span.source="interceptor"`. This is best-effort and only kicks in during the migration window.

## 8. Open Questions

1. **AI proxy as v1 dependency vs deferred.** Interceptor without proxy = observe-only; useful but not the full story. Should v1 ship the interceptor first and the proxy second, or both together? Recommend: ship interceptor first (observe-only), enable rewrites when the proxy is ready in a subsequent release. Avoids coupling timelines.
2. **Image signature preservation.** Users who require upstream Docker Inc. signature continuity on their base image won't get it through the mirror (mirror images are signed by Astro). Resolved via `agent.instrumentation: false`, which falls back to the upstream Docker Hub pull and produces an uninstrumented agent.
3. **Dynamic rule configuration.** v1 ships static rules in the interceptor bundle. v2 could fetch rules from a config endpoint at startup. Tradeoff: dynamic rules enable adding providers without rebuilding the mirror, but add a startup network dependency. Recommend: defer to v2.
4. **`--import` for ESM preload (Node 20+).** `--require` only loads CJS. Our interceptor bundle is CJS, so this works fine. But user code that does `import.meta`-based introspection or `node:test`-based modules may behave subtly differently with our `--require` loaded. Worth a soak test before GA.
5. **Bedrock SigV4 + rewriting.** Confirming whether the AI proxy can verify a SigV4 signature against a different host header. If not, Bedrock rewriting may need to be observe-only in v1.
6. **HTTP/2 from `undici`.** undici negotiates HTTP/2 to upstreams that support it. Our Dispatcher wrapping handles it transparently — but verify that streaming HTTP/2 responses tap cleanly.
7. **Application-level visibility on non-LLM hosts.** v1 emits spans only for rule-matched hosts; Beyla covers everything else at the network layer. If product later needs in-process visibility on a customer's own backend (e.g., to correlate traces across their service mesh), the path is to add a configurable allowlist via the object form of `agent.instrumentation`. Out of scope for v1 — flag here so we don't reinvent the surface later.

## 9. Implementation Notes

### 9.1 Repo layout

| Path | Contents |
|---|---|
| `packages/interceptor-rules/` (new) | Canonical rule set (`rules.yaml`) and generated `rules.json`. Shared across runtimes (see §11.5). |
| `modules/adapters/packages/node-interceptor/` (new) | TypeScript source of the Node interceptor; esbuild config; embeds `rules.json` at build; published as `@astropods/node-interceptor` |
| `packages/node-interceptor-image/` (new) | Dockerfile that bundles the npm artifact + multi-arch Go wrapper binary into the `astropods/node-interceptor` image |
| `packages/node-base-mirror/` (new) | CI pipeline that builds the patched Node base images served by `astro-registry`. For each tag in the mirror set, queries upstream metadata, generates a templated Dockerfile (§2.2), invokes BuildKit, and pushes the result. Scheduled daily; triggered on interceptor version bump. Self-contained Go module; future runtimes contribute a sibling `<runtime>-base-mirror/` package. |
| `apps/astro-registry/` (modified) | Extended to serve mirrored base images under their canonical Docker Hub paths (`library/node:*`). Auth and routing reuse existing infrastructure. |
| `apps/astro-cli/cmd/build_runner.go` (modified) | `prePullBaseImages` pulls recognized base-image references from `astro-registry` and tags them locally under the original Docker Hub reference. The build then resolves `FROM node:20` from the local cache. Dockerfile is not modified. Auth via the existing `getDockerRegistryAuth` flow. |
| `apps/astro-server/internal/deployment/spec_resolver.go` (modified) | Injects `ASTRO_AI_PROXY_URL`. Pre-built external `agent.image:` references are deployed as-is (no instrumentation; see §2.6). |
| `packages/astro-spec/spec.go` (modified) | Adds the polymorphic `Instrumentation` type and its `UnmarshalJSON`/`UnmarshalYAML`; adds `Instrumentation` field to the agent block. Runtime-neutral. |
| `packages/astro-spec/astropods.schema.json` (modified) | Adds `agent.instrumentation` as a `oneOf: [boolean, object]` field |
| `packages/astro-collector/internal/processor/astro/processor.go` (modified) | Optional dedup logic for migration window (§7.3) |
| `modules/adapters/packages/mastra/src/observability.ts` (modified) | Detects interceptor, no-ops if present |

Future runtimes add `modules/adapters/packages/<runtime>-interceptor/`, `packages/<runtime>-interceptor-image/`, and `packages/<runtime>-base-mirror/` — no changes to existing files. Each runtime's mirror pipeline is independent.

### 9.2 Tests

- **Interceptor unit tests** (`modules/adapters/packages/node-interceptor/`): patch installation across mocked undici / http versions; rewrite rule matching; SSE parser fuzz; failure containment.
- **Interceptor integration tests**: spin up real HTTP server mocking Anthropic / OpenAI, run a user agent, assert spans and rewrites.
- **Mirror pipeline unit tests** (`packages/node-base-mirror/`): tag enumeration, Dockerfile template generation against fixture upstream-metadata shapes, label correctness.
- **Mirror smoke test** (`make smoke-mirror`): for each mirrored base tag (debian, alpine, slim, bookworm × supported Node versions), build a hello-world Node image `FROM` the patched mirror image, run it, assert it boots and emits one span. Runs in CI on every interceptor change.
- **End-to-end test** (extension of `apps/astro-server/internal/e2e/`): deploy an agent that calls `https://httpbin.org/post`, assert collector receives a span with `http.url` = httpbin and `astro.span.source="interceptor"`.
- **AI-proxy contract test**: when the proxy ships, a shared contract test in both repos asserts header set and URL format match.

### 9.3 Telemetry budget

Rough order-of-magnitude:
- Bootstrap latency: < 50 ms per process start (one-time cost).
- Per-request overhead: < 100 μs p99 for non-matched requests (rule matching only — no wrapping, no span emitted; see §1.3). LLM requests add response-tap parsing — bounded by response size and the configured byte cap (§1.5).
- Memory: < 10 MB resident for the interceptor itself (no large caches).
- Span volume: 1 span per **matched** outbound HTTP request (LLM traffic only — non-LLM hosts emit nothing from the interceptor; Beyla covers them at the network layer). For a busy agent making ~10 LLM calls/s, 10 spans/s/pod sent to the collector. Existing collector + Galileo / Langfuse pipeline is sized for this with substantial headroom.

### 9.4 Security review touchpoints

- Mirror pipeline never executes user code. It operates exclusively on official upstream Node images pulled from Docker Hub, which we trust at the same level as Docker Hub itself. Upstream `ENTRYPOINT` / `CMD` are read from the image config (a static OCI manifest read, no `docker run`) and templated as JSON arrays into the patched Dockerfile, never as shell-interpreted strings.
- Mirror pipeline writes to internal `astro-registry`. Push credentials are scoped to the CI invocation and not shared with user code or with tenant build paths. The BuildKit invocation that produces the patched image runs in a sandboxed CI environment with no network access beyond pulling the upstream base from Docker Hub and pushing to our registry.
- Interceptor's `ASTROPODS_INTERCEPTOR_DISABLED` kill switch is readable by user code — by design (an escape hatch). Users cannot disable Beyla, which is the kernel-level safety net.
- Interceptor never writes plaintext credentials to logs. `Authorization` headers are pass-through; the interceptor records their presence (`astro.http.auth=true`) but not the value.

## 10. Rollout

| Phase | Scope | Acceptance |
|---|---|---|
| **0. Foundation** | Build interceptor module, `astropods/node-interceptor` image, and entrypoint wrapper binary. No call sites yet. | Manual test: a Dockerfile that `COPY`s the wrapper + interceptor into a vanilla `node:20` produces an image that runs and emits one span when given a fetch call. |
| **1. Base image mirror** | Build `packages/node-base-mirror` CI pipeline; produce patched `library/node:*` tags for the v1 mirror set (§2.1). Extend `astro-registry` to serve them under canonical Docker Hub paths. | Pulling `astro-registry/library/node:20` works end-to-end on amd64 and arm64; image carries the `org.astropods.interceptor.version` label; a hello-world Node image built `FROM` it emits one span. |
| **2. Cloud-builder mirror integration** | Configure cloud-builder BuildKit to use `astro-registry` as a Docker Hub mirror. Test against internal agents built via the GitHub integration. | A cloud build of `FROM node:20` produces an instrumented image without code changes to the cloud builder. |
| **3. CLI mirror integration** | Wire base-image pull redirection into `apps/astro-cli/cmd/build_runner.go` (`prePullBaseImages` pulls recognized references from `astro-registry` and tags them locally under the original reference; Dockerfile is unchanged). Behind a `--use-base-mirror` opt-in flag for the first release. | `astro deploy` from a workstation with `FROM node:20` produces the same instrumented image as the cloud-builder path. No user-side Docker config changes required; Dockerfile is not modified. |
| **4. Default-on** | Remove `--use-base-mirror` flag; redirection is the default for all CLI builds. `agent.instrumentation: false` is the user-facing opt-out. | All CLI and cloud builds against mirrored bases produce instrumented images by default; opt-out path tested. Builds against non-mirrored bases continue to work unchanged (uninstrumented). |
| **5. AI proxy enabled** | Set `ASTRO_AI_PROXY_URL` per environment. Rewrites take effect. | Proxy receives expected header set; upstream responses unchanged. |
| **6. Adapter deprecation** | `setupObservability` no-ops when interceptor present; log deprecation. | Documentation updated. |
| **7. Adapter removal** | Drop `setupObservability` from `@astropods/adapter-mastra`. | Major version bump; release notes call out the change. |

Phases 0–4 can ship before the AI proxy exists. Phase 5 depends on the AI proxy spec landing. Phases 1–2 can be parallel work after 0; phase 3 depends on phase 1; phase 4 depends on phases 2 and 3.

## 11. Multi-runtime extensibility

The spec is written Node-first because that's the v1 target, but the architecture is built so additional runtimes can slot in without redesign. This section pins down which pieces are shared, which are per-runtime, and the shape of work required to add a runtime. The specific patch points, bootstrap mechanisms, and library targets for any future runtime are intentionally out of scope here — they depend on investigation of that runtime's network stack and AI-SDK landscape, done when the runtime is prioritized.

### 11.1 What is shared across runtimes

These pieces are written once and used by every runtime's interceptor:

| Component | Why it's shared |
|---|---|
| **Base-image-mirror delivery pattern** (§2) | "Patch the upstream base image, serve it through our registry under the same canonical path" is the same pattern regardless of language. The Dockerfile-templating approach, the static Go entrypoint wrapper, the CLI's pull-redirection logic, and the cloud-builder's registry-mirror config are all runtime-neutral; only the patched payload and the binary names recognized by the wrapper change per runtime. |
| **`agent.instrumentation` spec field** (§3.1) | The polymorphic on/off/object switch is about whether *any* runtime interceptor is applied to this agent. No runtime-specific syntax. |
| **AI proxy wire contract** (§4) | HTTP-level. The proxy sees identical headers and request shape regardless of which language the rewriting happened in. `x-astro-upstream`, `x-astro-agent-id`, etc. are language-independent. |
| **Rewrite rule set** (§1.4) | Lives in a single source-of-truth file (`packages/interceptor-rules/rules.yaml` or similar) and is baked into each runtime's interceptor image at build time. Updating a rule once propagates to every runtime on next interceptor release. Avoids per-language drift. |
| **OTel span attributes** | `gen_ai.system`, `gen_ai.usage.input_tokens`, `astro.proxy.upstream`, etc. are part of the OTel `gen_ai` semantic conventions and have nothing to do with language. The collector's `astro` processor consumes the same attributes from any runtime's spans. |
| **Kill switches** (§6.2) | All four scopes (per-agent, per-deployment, cluster-wide proxy bypass, cluster-wide interceptor off) work identically across runtimes. `ASTROPODS_INTERCEPTOR_DISABLED=1` means "skip the interceptor" regardless of language. |
| **Failure model** (§6.1) | Fail-open semantics, span-emission policy, OTel collector path — all language-neutral. |

### 11.2 What is per-runtime

These are the only pieces that need new implementation when a runtime is added:

| Component | Why it's per-runtime |
|---|---|
| **Interceptor module** (§1) | Patches the runtime's network primitives in its own language. Same architecture (layered patching, response tap, rule matching), different code. The exact patch points depend on the runtime's network stack — chosen when that runtime is investigated. |
| **Bootstrap mechanism** | Whatever the runtime's equivalent of `NODE_OPTIONS=--require=...` is — most runtimes have some pre-import hook (env var, filesystem-based auto-load, or config file). The mirror's patched layer adds whatever that runtime needs. |
| **Binary detection** (wrapper, §2.3) | The list of recognized executable names differs per runtime (Node's list is `{node, nodejs}`; others will have their own). The wrapper binary's detection logic extends with each new runtime. |
| **Interceptor distribution image** | Each runtime ships its own image (`astropods/node-interceptor`, future runtimes get their own `astropods/<runtime>-interceptor`) with the runtime-specific bundle. |
| **Base image mirror set** | Each runtime that wants instrumentation defines its own set of upstream public base tags to mirror (Node → `library/node:*` variants; a future Python runtime would mirror `library/python:*` or similar). Each runtime ships its own `packages/<runtime>-base-mirror/` CI pipeline that builds the patched versions. |

### 11.3 Per-runtime mirror pipelines

Each runtime contributes a self-contained `packages/<runtime>-base-mirror/` package. The package owns:

- The list of upstream tags to mirror (the runtime's `library/<lang>:*` variants).
- The Dockerfile template that patches each tag (`COPY --from=` the interceptor bundle and wrapper into the right paths, set the ENTRYPOINT through the wrapper, label appropriately).
- The CI pipeline that runs daily and on interceptor version bump.

The pipelines are independent: there's no shared Go interface across them, no plugin registry, no central dispatch. Each pipeline operates on its own runtime's tags and pushes to `astro-registry` under the corresponding canonical paths. Adding a runtime is "write the new package"; it does not require modifying anything that exists.

### 11.4 Single multi-runtime wrapper binary

The static Go entrypoint wrapper (§2.3) does runtime detection on argv (recognizing `node` / `nodejs` today). Adding a new runtime extends the basename match list and adds whatever env-injection that runtime needs. If a runtime uses filesystem-based bootstrap rather than env injection, the wrapper changes nothing for it — the wrapper just identifies the binary and `exec`s the original command.

One wrapper binary per architecture continues to suffice; no proliferation of per-runtime entrypoint scripts.

### 11.5 Shared rule-set source of truth

Each runtime's interceptor needs the same rewrite rule set (which hosts to match, where to route, which extractor to use). To avoid drift:

- Rules live in `packages/interceptor-rules/rules.yaml` (canonical) and a generated `rules.json` for runtimes that prefer JSON.
- Each runtime's interceptor image embeds `rules.json` at build time (the Node image bundles it into the interceptor bundle; future runtimes embed it however their packaging works).
- Rule edits land in `rules.yaml`; CI rebuilds every runtime's interceptor image on next release. There is no path by which two runtimes see different rule sets at the same interceptor version.

Extraction strategies (`anthropic_messages`, `openai_chat`, `openai_responses`, `openai_embeddings`, `bedrock_invoke`) need a per-runtime implementation in each interceptor — the JSON / SSE parsing logic itself doesn't cross languages — but the *list* of strategies and their names is shared via the same rule file.

### 11.6 Adding a future runtime

When a new runtime is prioritized, the work breaks down to:

1. **Investigate the runtime's network stack** — identify the equivalents of `undici` / `http` / `https`: the libraries the AI SDKs of interest actually use, and where the natural patch points are. This is the substantive engineering work; the rest is wiring.
2. **Write the interceptor module** in the target language. Same architecture (layered patching, response tap, rule matching), language-specific implementation. Embed `rules.json`.
3. **Pick the runtime's bootstrap mechanism** — the equivalent of `NODE_OPTIONS=--require`. Most modern runtimes have a pre-import hook of some kind; choose the one that runs before user code, requires no developer action, and survives `child_process` / subprocess equivalents.
4. **Build the distribution image** — `astropods/<runtime>-interceptor:vX` containing the bundle plus any per-runtime artifacts the bootstrap mechanism needs. Multi-arch, same shape as the Node image.
5. **Define the mirror set and build pipeline** (§11.3) — a new `packages/<runtime>-base-mirror/` package that lists the upstream base tags to mirror, templates a patched Dockerfile per tag, and pushes the results to `astro-registry`.
6. **Extend the wrapper** — add the runtime's binary basenames to the §2.3 detection logic. If the runtime requires argv-time env injection, add that too. If the bootstrap is purely filesystem-based, the wrapper just identifies the binary and exec's.
7. **Sunset any pre-existing per-language manual observability adapter** through the same three-release deprecation cycle as the Mastra adapter (§7.1), gated on detecting the interceptor via the runtime-appropriate equivalent of `globalThis[Symbol.for("astropods.interceptor")]`.

Steps 5–7 are mechanical. Everything else — spec field, AI proxy, rule set, OTel pipeline, kill switches, CLI redirection logic, cloud-builder registry-mirror config — is reused as-is. The architecture's purpose is to keep step 2 (the language-specific interceptor) as the only substantial new code per runtime.

### 11.7 Implications for naming

The current spec uses `astropods/node-interceptor` for the interceptor image and `packages/node-base-mirror` for the mirror build pipeline. These names are deliberately runtime-scoped so future runtimes can be added without collisions:

- Per-runtime images get distinct names: `astropods/node-interceptor`, and `astropods/<runtime>-interceptor` for future additions.
- Per-runtime mirror pipelines get distinct package names: `packages/node-base-mirror`, and `packages/<runtime>-base-mirror` for future runtimes.
- The user-facing spec field stays `agent.instrumentation` (no `agent.nodeInstrumentation`). The field's semantics are "Astro auto-instruments this agent in whatever runtime it ends up being."
- Env vars use `ASTROPODS_INTERCEPTOR_*` (singular, no runtime in the name). The same disable flag works regardless of language.

If we ever need a runtime-specific knob, it lands as a field inside `agent.instrumentation`'s object form (e.g., `agent.instrumentation.runtimes: { <runtime>: false }`) rather than a new top-level field — keeping the surface area small.
