# Interactive Rendering & Elicitation Specification

**Version:** 0.17 (Draft)
**Status:** Draft, for review
**Date:** 2026-06-29

## Overview

Agents need to render structured content to the user mid-conversation and, when required, block on a structured response. MCP calls this *elicitation* (the agent asks the user for typed input); frameworks expose it as human-in-the-loop (Mastra `suspend`/`resume`, LangGraph `interrupt`). Today the chat path carries only plain text end to end.

This spec adds one framework-agnostic primitive, the **Renderable**, that flows from agent to user across both transport layers and renders on every surface (web, Slack, future targets). In v1 a Renderable is a **blocking interaction**: the agent asks, and the conversation gates until the user answers, declines, or cancels. Its most common application is a **tool-call approval**: the agent proposes a tool call and the user approves, edits, or denies it. The same primitive is designed to extend to passive display and non-blocking interactions in a later phase, alongside the component render strategy, without breaking changes.

We take two positions:

- **Keep the transport, adopt MCP's vocabulary.** We keep the gRPC messaging SDK as transport and add an MCP-compatible *data model*.
- **Light-touch on the developer, full on capability.** Adapters normalize each framework's native HITL primitive into the Renderable, so an existing agent works with little or no Astro-specific code.

## Terminology

- **Renderable.** Agent-emitted structured content: a data schema (with optional inline `x-ui` render hints), optional prefilled value, an optional `intent` tag, and a set of allowed response actions.
- **Intent.** An optional open-vocabulary tag on a Renderable. `tool_permission` is the recognized value (approve / edit / deny over a proposed tool call); it drives approval-card rendering and outcome metrics. Unset for a plain elicitation.
- **Render strategy.** How a Renderable is drawn. v1 ships **form** (declarative: schema → host-native widgets). The **component** strategy (sandboxed agent-authored UI, ie MCP apps) waits for a later phase.
- **Interaction.** A Renderable the user can answer with a `RenderableResponse`. In v1 every Renderable is an interaction, every interaction blocks, and every interaction must offer `CANCEL` so the user can always escape.
- **Blocking queue.** Pending blocking interactions form a FIFO queue (ordered by `seq`); the conversation gates on the head until it is answered, declined, or cancelled. v1 has only blocking interactions, so every pending interaction sits in this queue.
- **Implicit cancel.** On an uncontrolled composer (Slack, Discord, Teams) the user can ignore a pending interaction and just type. That message resolves the head-of-queue interaction as `CANCEL`, then is handled as a normal new turn. On a controlled composer (web) it cannot happen: the composer is replaced by the form until the user answers or dismisses.
- **RenderableResponse.** The user's reply to an interaction: an action plus optional payload.
- **Two layers.** (1) browser ↔ astro-server ↔ messaging sidecar over HTTP/SSE (astro-server authenticates and proxies; the sidecar owns chat persistence); (2) sidecar platform adapter ↔ agent over the gRPC messaging SDK. A Renderable traverses both and must survive page reload; surviving agent restart is best-effort and framework-dependent (see resumption).

## Goals

- One primitive for blocking elicitation in v1, designed to extend to passive and non-blocking display later, render-strategy-extensible.
- **Portable data model:** full JSON Schema (data) with optional inline render hints, so the *same* Renderable renders in the web client, in Slack, and in future targets, none of which share a renderer. MCP elicitation's flat-primitive schema is an accepted *subset profile*, so MCP-native agents pass through unchanged.
- Superset action model spanning MCP (accept/decline/cancel) and LangGraph (accept/edit/respond/ignore), with edit-in-place via a prefilled value.
- **Tool-call approval as the primary use case.** A permission request is a kind of elicitation; the primitive models approve / edit / deny over a proposed call and records the outcome as raw data for model-performance metrics.
- Backward-compatible, additive wire changes; a defined outcome on every surface (graceful degrade for free-text-tolerant asks, a typed failure for strict ones).
- Adapter-level normalization so Mastra/LangGraph/MCP agents work with minimal developer effort and no auto-modification of their agent.
- **Durable answers, best-effort resume.** A Renderable and its response are persisted in the deployment-local store (the messaging sidecar's SQLite, on a persistent volume; see astropods/messaging#61), never the central platform DB. A pending interaction waits indefinitely while the thread is open (no timeout, no auto-drop). Resuming the *agent* after it restarts depends on the framework's checkpointer, not on the transport; we support frameworks that already implement durable resume (Mastra/LangGraph) and do not supply it for those that don't.
- Blocking interactions queue (FIFO) and are always cancelable.

## Non-Goals (v1)

- **Component render strategy** (sandboxed iframe + postMessage/JSON-RPC bridge, à la MCP Apps). Reserved; the model admits it later without breaking changes.
- Arbitrary agent-authored UI code execution in any client.
- **Non-blocking and passive Renderables.** v1 ships blocking elicitation only. The data model and wire format leave room for both, and they arrive with the component render strategy in a later phase.
- **Cross-restart recovery for non-checkpointed (live-session) agents.** An in-process `elicit()`/`ask_user` await holds state in the running process; a restart abandons that specific in-flight call (the response is still stored and visible). Recovering it would require conversation **replay**, which is out of scope. Checkpointed frameworks (Mastra/LangGraph) resume after restart (best-effort, see resumption).
- **Scale-to-zero during a pending wait (Model B).** v1 keeps the agent running while an interaction is pending (Model A); the stream is the agent's baseline connection, not a per-interaction one, so this is low-cost. Tearing an idle agent down and waking it on response is a future path (see resumption), not precluded.
- **Collecting sensitive data (passwords, API keys, secrets).** Elicitation MUST NOT be used to gather credentials or other sensitive values: the response is persisted and may transit logs and traces. Secure secret handling is out of scope for v1 (see Security).
- Voice/audio elicitation UX beyond degradation to a spoken question.
- Interaction expiry/timeouts. Pending interactions never expire while the thread is open; the user clears one by answering or cancelling, not by timeout. A non-empty queue gates sends until then (intentional).

## Design

### The unified data model

A Renderable is the single content type. In v1 every Renderable is a blocking interaction: `allowed_actions` is non-empty, the conversation gates on it, and it joins the FIFO queue. `allowed_actions` **must include `CANCEL`** (or `DECLINE`) so the user can always clear a blocking ask, and the client also renders a dismiss that resolves the head as `CANCEL`. The data model leaves room for passive (`allowed_actions` empty) and non-blocking Renderables later; v1 does not emit or render them.

The action vocabulary is the cross-framework superset. **Edit-in-place reuses `SUBMIT`** over a prefilled `value` rather than adding an action. This collapses LangGraph's four-action model:

| Action | Payload | MCP | LangGraph |
|--------|---------|-----|-----------|
| `SUBMIT` | `content` (object matching data schema) | accept | accept; edit (with prefilled `value`) |
| `RESPOND` | `text` (free-form string) | n/a | respond |
| `DECLINE` | none | decline | (folds into ignore) |
| `CANCEL` | none | cancel | ignore |

`allowed_actions` per Renderable mirrors LangGraph's `allow_*` gating: a plain elicitation offers `[SUBMIT, CANCEL]`; an editable proposal that also accepts free text offers `[SUBMIT, RESPOND, CANCEL]`. Including `RESPOND` makes an interaction **free-text-tolerant**: on a surface that can't render the form, the answer may come back as `RESPOND{text}`. Omitting it makes the interaction **strict**: the response must match the schema or fail explicitly, never free text (see the degradation and failure contract). Strict is the right choice when the response is consumed by deterministic code outside the agent loop.

The data model is **MCP-compatible by construction**: every MCP `requestedSchema` is a valid flat JSON Schema, so an MCP elicitation maps to a Renderable with `kind=FORM`, the flat schema in `data_schema_json`, and `allowed_actions=[SUBMIT, DECLINE, CANCEL]`.

### Tool-permission and approval workflows

A permission request is a kind of elicitation, and the most common one: the agent proposes a tool call and the user approves, edits, or denies it. The primitive models it directly, so this is a first-class use of the same machinery, not a separate feature.

**Mapping.** `data_schema_json` is the tool's input schema; `value` is the proposed arguments, so the card opens prefilled. The outcome reuses the action model:

- **Approve as-is** → `SUBMIT` with `content` equal to `value`.
- **Approve with edits** → `SUBMIT` with edited `content` (the edit-in-place pattern; maps to a framework's modified-input return).
- **Deny** → `DECLINE`.
- **Cancel / defer** → `CANCEL`.

Permission asks are **strict** (no `RESPOND`): the consumer is the deterministic permission gate, not the model, so an incapable surface yields `UNSUPPORTED` and the gate defaults to deny.

**Intent.** The Renderable sets `intent: "tool_permission"`. A client MAY render a recognizable approval card (the proposed call plus Approve / Edit / Deny, one-tap on Slack), and the platform groups these for metrics.

**Native hooks.** First-class adapters map each framework's tool-approval primitive onto a `tool_permission` Renderable: Claude Agent SDK `PreToolUse`/`PermissionRequest`, Mastra `requireApproval` (`approveToolCall()` / `declineToolCall()`), LangGraph interrupt-before-tool. The adapter maps the response back to allow / allow-with-modified-input / deny.

**Outcome data (metrics).** astro-server records each outcome (approved, approved-with-edits, denied, cancelled) on the interaction row, keyed by `intent`, and emits it to observability (the OTel → Langfuse pipeline); the adapter, which knows the tool, enriches the telemetry with the tool identity. v1 captures the **raw outcome data**; approval/denial-rate metrics per tool and per agent are interpretive work layered on later.

**Standing rules (extension).** "Approve and don't ask again" maps to a persistent permission rule (the Claude Agent SDK's `PermissionRequest` supports this). Out of scope for v1, noted as a natural extension: an extra outcome on the card the adapter turns into a standing rule.

### Schema standard

A Renderable carries one document, the **data schema** (JSON Schema 2020-12), which both validates the response and drives rendering. Presentation is expressed by optional inline `x-ui` **render hints** on individual properties (advisory, honored per surface); v1 has no separate layout document. Expansive layout (groups, tabs, conditional visibility) is deferred to the component render strategy.

The supported types, keywords, render hints, and validation rules are defined in the companion [Renderable Schema Specification](renderable-schema-spec.md). Two fidelity tiers matter here:

- **Core profile** (a flat object of primitives, enums, and multi-enums) renders on every surface and equals MCP elicitation's flat schema, so MCP elicitations pass through unchanged.
- **Extended profile** (nesting, arrays, `oneOf`/`anyOf`, conditionals) renders on rich surfaces such as web and degrades on Core-only surfaces such as Slack, per the degradation contract.

See [Examples](#examples) for concrete Renderables across the common capabilities.

### Wire layer: gRPC messaging SDK

New `modules/messaging/proto/astro/messaging/v1/renderable.proto`:

```protobuf
enum RenderKind {
  RENDER_KIND_UNSPECIFIED = 0;
  RENDER_KIND_FORM = 1;          // declarative (v1)
  // RENDER_KIND_COMPONENT = 2;  // sandboxed component (future)
}

enum RenderableAction {
  RENDERABLE_ACTION_UNSPECIFIED = 0;
  RENDERABLE_ACTION_SUBMIT = 1;
  RENDERABLE_ACTION_DECLINE = 2;
  RENDERABLE_ACTION_CANCEL = 3;
  RENDERABLE_ACTION_RESPOND = 4;
  RENDERABLE_ACTION_UNSUPPORTED = 5;  // system-emitted: a strict ask reached a surface that can't render it
}

message Renderable {
  string id = 1;                                 // correlation id (agent-generated)
  RenderKind kind = 2;
  string message = 3;                            // prompt / title text (markdown)
  string data_schema_json = 4;                   // full JSON Schema; render hints inline via "x-ui"
  string value_json = 5;                         // optional prefilled / proposed data
  repeated RenderableAction allowed_actions = 6; // must include CANCEL or DECLINE (v1: always blocking)
  string intent = 7;                             // optional; open vocab, "tool_permission" recognized
}

message RenderableResponse {
  string id = 1;                                 // correlates to Renderable.id
  RenderableAction action = 2;
  string content_json = 3;                       // iff action == SUBMIT; matches data schema
  string text = 4;                               // iff action == RESPOND
}
```

Enum values are prefixed (`RENDERABLE_ACTION_*`, `RENDER_KIND_*`) per `buf lint`. The data schema and values travel as JSON strings rather than proto-modeled types, which keeps the wire out of JSON Schema's business and forward-compatible with MCP schema evolution. `message` is rendered as **markdown** on surfaces that support it (plain text elsewhere), which covers light rich display (code blocks, a diff as fenced code, links) without a dedicated content field.

Additions to existing oneofs (next free field numbers verified against current protos):

- `AgentResponse.payload` (`response.proto`): highest current field is `13`, so:
  ```protobuf
  Renderable renderable = 14;                    // agent emits content to render
  ```
- `PlatformFeedback.feedback` (`feedback.proto`): highest current field is `10`, so:
  ```protobuf
  RenderableResponse renderable_response = 11;   // user's response (rides existing feedback channel)
  ```

The Renderable extends the agent-output union (`AgentResponse.payload`); the response extends the user-action union (`PlatformFeedback.feedback`), which already carries button clicks (`ButtonClick`, 7) and free-text (`TextFeedback`, 10). No new RPCs. All additions are proto3-additive; older peers ignore unknown fields.

### Agent-side adapter (`modules/adapters/packages/core`)

Existing `AgentAdapter` and `StreamHooks` stay unchanged, because rendering is an agent-*initiated* request rather than a hook. We add to `StreamOptions`:

- `render(renderable): Promise<RenderableResponse>`. Emits a blocking Renderable and resolves with the user's response: `SUBMIT`, `DECLINE`, `CANCEL`, or (only if allowed) `RESPOND` (see resumption for how the resolution arrives, alive or after restart). For a strict ask that reaches a surface which can't render it, the promise rejects with a typed `UNSUPPORTED` failure rather than resolving with free text, so out-of-loop callers can branch deterministically.
- `elicit(message, dataSchema, opts?): Promise<RenderableResponse>`. A thin MCP-elicitation-shaped convenience over `render`.

`MessagingBridge` gains `sendRenderable(conversationId, renderable)`, an in-memory `id → pending promise` map for live awaits, and an optional `resume(conversationId, response)` entry point an adapter wires to its framework's native resume. Existing adapters keep working untouched (light-touch).

### Resumption

A blocking response may arrive in seconds, or after the user steps away for an hour. This section defines how it reaches the agent, and what happens if it never does.

**Correctness does not depend on a held-open connection.** The interaction row in the sidecar's durable SQLite store (a persistent volume; see astropods/messaging#61) is the source of truth, and delivery is idempotent (keyed by `Renderable.id`). The agent's stream is the *fast path*, not a requirement: if it drops, nothing is lost; the response is redelivered when the agent reconnects.

**Transport shape.** The agent dials the sidecar and holds one long-lived, multiplexed stream; the sidecar reaches the agent only over it and drops a send when no stream is registered. The store and the agent are **co-located in the same pod**, so delivery is in-pod and astro-server is only a proxy. v1 assumes the agent stays running while an interaction is pending (Model A); scale-to-zero (Model B) is a future path (see non-goals).

**Delivery (agent running).** The sidecar records the response in SQLite, then delivers it to the local agent over the open stream. The bridge resolves the in-memory promise, or hands it to the adapter's `resume` (checkpointed frameworks that yielded the turn). The user may answer **indefinitely** while the thread is open; nothing times out. (gRPC keepalive on the in-pod stream is an optional optimization that trims reconnection latency; the SDK already auto-reconnects with buffered sends, and the durable row makes a dropped stream a non-event.)

**Recovery (agent restart).** The in-memory promise dies with the process, but the SQLite row survives (persistent volume). On the agent's next connect, the sidecar delivers any answered-but-undelivered responses from SQLite (a local step, mirroring the existing history backfill), and the agent applies each via the framework `resume` entry point. There is no astro-server redelivery and no cross-pod back-channel: both the store and the agent live in the sidecar's pod.

| Agent type | Resumes after a restart? |
|------------|--------------------------|
| Checkpointed (Mastra, LangGraph) | Yes; state rebuilds from the framework checkpointer, via `Command(resume=...)` / `run.resume({resumeData})` |
| Bring-your-own | Yes, if the developer wires the `resume(conversationId, response)` entry point (opt-in) |
| Non-checkpointed live-session (in-process `elicit()`/`ask_user`) | Only while the process lives; a restart abandons the in-flight await (the response stays stored and visible; see non-goals) |

**Abandonment (the user never answers).** A pending interaction is an expected resting state, not an error. It persists with no expiry, gates only its own thread (other threads are unaffected), and re-renders on return from the durable store. The user can always clear it by answering, declining, or cancelling (`CANCEL` is mandatory; on an uncontrolled composer, an implicit cancel does the same). For a non-checkpointed live-session agent the cost of an unanswered ask is a parked in-process await bound to the process lifetime, so where abandonment is likely the checkpointed yield-the-turn path is preferable.

### Adapter normalization

First-class adapters map each framework's native primitive onto `render`/`resume`, so developers keep their existing code.

| Framework | Developer writes | Adapter maps to |
|-----------|------------------|-----------------|
| Claude Agent SDK | nothing extra to turn its tool-permission prompts into elicitations | the adapter wires the SDK's **`PreToolUse`/`PermissionRequest` hook** to a `tool_permission` `render()` (approve / edit / deny). MCP `elicitInput` from embedded servers is SDK-internal (surfaced only as a read-only notification) and is **not** routed; structured elicitation uses the in-tool `render()`/`elicit()` path. |
| Mastra | `suspend()` with Zod `suspendSchema` (already does) | Zod→JSON Schema; `resumeData` ← response; resume via `run.resume` |
| LangGraph | `interrupt(HumanInterrupt)` | `allow_*` → `allowed_actions`; `args` → `value` for edit; resume via `Command(resume=...)`. (Multiple interrupts in one super-step → enqueue all → resume once with the response map) |
| AI SDK / any tool-using agent | imports the provided **opt-in** `ask_user` tool and adds it to their tool list | model-driven elicitation via the happy-path await; cross-restart recovery only if the agent checkpoints |
| Bring-your-own | one `render()` / `elicit()` call (+ optional `resume`) | explicit escape hatch |

**`ask_user` is a provided, opt-in helper** (a tool definition plus a handler that calls `render()`) that a developer imports and adds in one line as a convenience. It doubles as the worked example for bring-your-own elicitation. Each adapter ships per-framework docs for wiring elicitation; the Claude Agent SDK adapter additionally routes the SDK's tool-permission hook (`PreToolUse`/`PermissionRequest`) to a `tool_permission` elicitation with no extra developer code. That hook is its primary integration, since MCP `elicitInput` is not interceptable.

### Platform-side adapter (`modules/messaging/internal/adapter`)

- `AdapterCapabilities` gains `SupportsDeclarativeForms bool` (and reserved `SupportsEmbeddedComponents bool`, false in v1). Web → true; Slack → true (inline + button→modal); audio-only → false.
- `HandleAgentResponse` handles the `renderable` payload: web → emit an SSE interaction event; Slack → inline blocks for single-field interactions, or a button→modal handshake for multi-field forms (see Slack rendering).
- **Slack rendering.** Slack cannot open a modal proactively: `views.open` needs a `trigger_id` that exists only ~3s after a *user* interaction, and an agent-initiated Renderable has none. So:
  - **Single-field interactions** (one choice, confirm, or short text) render inline as buttons or a `static_select`; the selection rides a block action → `PlatformFeedback`.
  - **Multi-field forms** use the **button → modal → `view_submission`** handshake: the agent posts a button, the user's click supplies a `trigger_id`, the adapter opens a modal, and the submission delivers all values atomically. This does cover the blocking scenario (the agent's turn stays parked until the submission arrives); its only costs are one extra click and the trigger lifecycle (a dismissed modal needs a fresh click to reopen). Inline message blocks cannot atomically submit a multi-field form, so the modal is required there, not optional.
  - A degraded Slack experience is expected; we do not target web parity.
- **Degradation and failure contract.** The platform guarantee is narrow: it never silently drops a Renderable and never silently mis-types a response. It always returns a defined outcome, and it never coerces free text into `SUBMIT{content}`. Beyond that, the developer chooses the behavior through `allowed_actions`:
  - **Free-text-tolerant** (`RESPOND` included): on a surface that can't render the form, the adapter renders the prompt as text and returns the answer as `RESPOND{text}`. Use this when the consumer (typically the model in the agent loop) can handle prose.
  - **Strict** (`RESPOND` omitted): the adapter MUST NOT coerce to text. If it can't render the form, it returns a distinct, typed `UNSUPPORTED` outcome (surfaced as a rejected `render()`), separate from a user `DECLINE`/`CANCEL`, so the consumer can tell "couldn't ask" from "user said no." Use this when the response is consumed by deterministic code that requires the schema shape, including an elicitation handled outside the agent loop.
  - A `render()` caller therefore sees exactly one of: a valid `SUBMIT{content}`, an explicit `DECLINE`/`CANCEL`, `RESPOND{text}` (only if allowed), or a typed `UNSUPPORTED` failure (strict only). Never a surprise string.

  The contract yields three outcome paths:

  ```mermaid
  flowchart TD
      R["Renderable reaches a surface"] --> Q{"Can the surface<br/>render the form?"}
      Q -->|yes| S["Render the form;<br/>user responds"]
      S --> O1["SUBMIT content<br/>(or DECLINE / CANCEL)"]
      Q -->|no| M{"RESPOND in<br/>allowed_actions?"}
      M -->|"yes: free-text-tolerant"| T["Render prompt as text"]
      T --> O2["RESPOND text"]
      M -->|"no: strict"| O3["UNSUPPORTED<br/>(typed failure)"]
  ```
- Inbound: the adapter receives the user's reply (web: POST from client; Slack: block action / view submission) and emits `PlatformFeedback.renderable_response` via the feedback handler. On an uncontrolled composer, a normal message arriving while a blocking interaction is pending is an **implicit cancel** of the head (emitted as a `CANCEL` `renderable_response`) before that message is handled as a new turn.

**Cross-platform note.** The *data model* is portable; the *form* strategy renders on any surface because it's declarative, but render-hint fidelity is best-effort per surface (web honors `x-ui` hints; Slack maps what it can and ignores the rest). The planned *component* strategy will be web-only (needs a webview). That asymmetry is why form is the default and the data model stays renderer-agnostic. Chat surfaces also vary widely in widgets, option caps, and atomic multi-field submit (see the schema spec's [Surface Capability Limits](renderable-schema-spec.md#24-surface-capability-limits)), so even Core schemas render best-effort there, not identically.

### HTTP/SSE layer: astro-server + client

**Server.** The interaction surface rides the deployment-local chat contract the messaging sidecar now owns (`/api/chat/*`; see astropods/messaging#61). astro-server authenticates and proxies `/chat/*` to the sidecar and stores nothing itself.

When a Renderable arrives from the agent, the sidecar's web adapter persists it (see persistence) and emits a new SSE event alongside `chunk`/`finish`/`error`:

```
event: interaction
data: {"id":"...","kind":"form","message":"...","dataSchema":{...},"value":{...},"actions":["submit","cancel"]}
```

(The sidecar parses the agent-provided `data_schema_json` and re-embeds it as a JSON object; malformed JSON is rejected. Enum actions lowercase to `submit`/`decline`/`cancel`/`respond`.)

The response endpoint is a sidecar route under the chat contract, proxied by astro-server:

```
POST /api/chat/conversations/{conversationId}/interactions/{interactionId}
body: {"action":"submit","content":{...}}   // or {"action":"respond","text":"..."} | decline | cancel
```

The sidecar **validates `content` against the stored data schema** (a Go JSON Schema 2020-12 validator whose semantics match the client validator, see open questions), verifies the `interactionId` belongs to the conversation, is `pending`, and comes from the conversation's authorized user, then records the response in SQLite and delivers it to the local agent. The endpoint is **idempotent** by `interactionId`: a re-POST after reload returns the recorded result rather than double-submitting.

**Client (`apps/astro-client`).**

- `transport.ts` gains an `onInteraction` callback for the `interaction` event; following the existing pattern, the **hook** (`use-deployment-chat.ts`) patches the interaction into the TanStack Query cache (transport itself never touches the cache).
- Interactions are a separate `seq`-interleaved entity, not chat messages (the chat message model is flat text today). They surface to assistant-ui as a synthesized `interaction` part rendered by a JSON Forms renderer (`@jsonforms/react` with an Astro-themed renderer set, a new dependency). Buttons reflect `allowed_actions`; fields prefilled from `value`.
- **Turn/input gating** is the existing `assistantStreaming`/`threadIsRunning` mechanism, *not* `ChatComposerState` (which is deployment-health: ready/paused/stopped/…). We add `waiting-for-input` ≡ **the pending queue is non-empty**, modeled alongside `threadIsRunning`. While set, the composer is replaced by the head-of-queue interaction form. v1 presents one interaction at a time (the head); rendering all pending cards at once is a non-precluded client option (id-correlation makes answer order irrelevant).
- **A pending interaction gates user input, not agent output.** Assistant messages that arrive while an interaction is pending render in `seq` order in the timeline; the head-of-queue form stays in the composer. The platform does not buffer or suppress agent output. (Custom surfaces receive the same `seq`-ordered stream of messages and interactions and choose their own presentation.)
- On submit the form enters a **pending/disabled** state (it is not optimistically cleared); it clears on server ack and advances the queue. On validation rejection it re-enables in place with the error. Decline/cancel/respond follow the same ack-then-advance flow.

### Persistence

In-flight interactions must survive reload and agent restart. Persistence lives in the **messaging sidecar's SQLite store** on a persistent volume (astropods/messaging#61), not the central platform DB; keeping chat content and interaction responses (user input) deployment-local is a data-minimization requirement. The interaction table is a third table alongside the sidecar's `conversations` and `messages`, sharing their contiguous `seq` space so interactions interleave with messages by order:

```sql
CREATE TABLE IF NOT EXISTS interactions (
    id              TEXT    NOT NULL,   -- Renderable.id
    conversation_id TEXT    NOT NULL,
    user_id         TEXT    NOT NULL,
    seq             INTEGER NOT NULL,   -- shared ordering space with messages
    render_json     TEXT    NOT NULL,   -- the Renderable
    status          TEXT    NOT NULL DEFAULT 'pending',  -- pending|submitted|declined|cancelled|responded
    response_json   TEXT,               -- the RenderableResponse, once answered
    created_at      INTEGER NOT NULL,
    responded_at    INTEGER,
    PRIMARY KEY (conversation_id, id),
    UNIQUE (conversation_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_interactions_pending
    ON interactions(conversation_id, seq) WHERE status = 'pending';
```

`seq` is allocated `MAX(seq)+1` per conversation, shared with messages. Store methods: `AppendInteraction`, `RecordInteractionResponse`, `ListInteractions`, `PendingInteractions` (the FIFO queue, ordered by `seq`). A conversation fetch interleaves messages and interactions by `seq` and returns the ordered pending queue, so a reloaded client re-renders open forms and re-enters `waiting-for-input`. A non-empty queue gates sends.

**Durability and restart.** The persistent volume survives a pod reschedule, so the row and the user-facing card persist directly. The agent's *resumption* rests on its framework checkpointer (a separate, framework-owned store): on restart the agent rehydrates there and picks up the response the sidecar delivers on reconnect. Pending interactions are not written to Langfuse; Langfuse (the message-history backstop, astropods/messaging#61) rebuilds the conversation text if the store is ever empty, while the checkpointer re-creates a still-pending ask. **Soft-deleting a conversation cancels its pending interactions** (resolved as `CANCEL` to the agent, so a suspended agent does not hang) and hides them.

### Security (v1, declarative)

No agent code executes. The host renders host-native widgets, so the iframe and sandbox concerns of the component strategy do not apply. Requirements:

- Show provenance (which agent is asking); never auto-submit; every blocking interaction MUST offer `CANCEL` (or `DECLINE`), and the client always renders a dismiss that resolves the head as `CANCEL`, so a conversation can never wedge.
- **No sensitive data.** Elicitation MUST NOT be used to collect passwords, API keys, or other secrets. Responses are persisted and may appear in logs or traces, so sensitive values do not belong in them. Secure secret handling is out of scope for v1.
- Validate `content` against the data schema on both client and server.
- Server authorizes the response: the `interactionId` must belong to the conversation, be `pending`, and come from the conversation's authorized user; the endpoint is idempotent.
- **Authorized responder (multi-user surfaces).** Each interaction records its authorized responder (the conversation's user). On a shared surface (a Slack channel or thread, Discord, Teams) the prompt and any button are visible to others, and anyone may click to open a modal. The adapter MUST check the responder's platform identity (for example the Slack `view_submission`/`block_actions` `user.id`) against the authorized responder and reject mismatches (drop the response, optionally with an ephemeral notice) before emitting `renderable_response`. A wrong-user submission never reaches the agent.
- Rate-limit interactions per conversation.

The future component strategy will require the MCP Apps isolation model (different-origin sandboxed iframe, restrictive CSP, origin-validated bridge). Out of scope here; noted so the engine boundary leaves room for it.

### Versioning & compatibility

- All proto changes are additive (`renderable` 14, `renderable_response` 11, `Renderable.intent` 7); no new RPCs; old agents/sidecars ignore unknown fields. No breaking change to existing text chat.
- `SupportsDeclarativeForms` gates surfacing; degradation covers incapable platforms.
- Messaging SDK version bump from `@astropods/messaging` 0.0.7 to 0.1.0, since this adds protocol surface.
- Persistence rides the sidecar SQLite store + Langfuse history backstop (astropods/messaging#61); no central-DB schema change.

## Examples

Each example shows the Renderable in the client/SSE JSON shape (`actions` lowercases `allowed_actions`; `id` omitted) and the resulting `RenderableResponse`. Types and keywords are defined in the [Renderable Schema Specification](renderable-schema-spec.md).

**Text input** (MCP-style single field):

```json
{ "kind": "form", "message": "What's your GitHub username?",
  "dataSchema": { "type": "object", "properties": { "username": { "type": "string" } }, "required": ["username"] },
  "actions": ["submit", "cancel"] }
```

Response: `{ "action": "submit", "content": { "username": "octocat" } }`.

**Single-select from preset options** (the Claude Code choice):

```json
{ "kind": "form", "message": "Which environment should I deploy to?",
  "dataSchema": { "type": "object",
    "properties": { "env": { "type": "string", "enum": ["dev", "staging", "prod"],
      "enumNames": ["Development", "Staging", "Production"] } },
    "required": ["env"] },
  "actions": ["submit", "cancel"] }
```

Web renders a radio group or one-tap buttons; Slack a `static_select` or button row. Response: `{ "action": "submit", "content": { "env": "prod" } }`.

**Pick one or write your own** (preset list with a free-text escape):

```json
{ "kind": "form", "message": "Pick a base branch, or tell me another.",
  "dataSchema": { "type": "object", "properties": { "branch": { "type": "string", "enum": ["main", "develop"] } }, "required": ["branch"] },
  "actions": ["submit", "respond", "cancel"] }
```

`respond` lets the user supply a branch not in the list. Response: `{ "action": "respond", "text": "release/2026-07" }`.

**Confirmation** (yes/no):

```json
{ "kind": "form", "message": "Delete 4 stale deployments?",
  "dataSchema": { "type": "object", "properties": { "confirm": { "type": "boolean", "title": "Yes, delete them" } }, "required": ["confirm"] },
  "actions": ["submit", "cancel"] }
```

Renders as a checkbox or a confirm/cancel pair. Response: `{ "action": "submit", "content": { "confirm": true } }`.

**Multi-select**:

```json
{ "kind": "form", "message": "Which regions should this run in?",
  "dataSchema": { "type": "object",
    "properties": { "regions": { "type": "array", "uniqueItems": true,
      "items": { "type": "string", "enum": ["us-east", "us-west", "eu", "apac"] } } },
    "required": ["regions"] },
  "actions": ["submit", "cancel"] }
```

Web renders checkboxes. Response: `{ "action": "submit", "content": { "regions": ["us-east", "eu"] } }`.

**Structured form** (multiple fields):

```json
{ "kind": "form", "message": "Confirm your contact details.",
  "dataSchema": { "type": "object",
    "properties": {
      "name": { "type": "string", "title": "Full name" },
      "email": { "type": "string", "format": "email" },
      "age": { "type": "number", "minimum": 18 } },
    "required": ["name", "email"] },
  "actions": ["submit", "cancel"] }
```

Response: `{ "action": "submit", "content": { "name": "Mona", "email": "mona@example.com", "age": 31 } }`.

**Edit a proposal** (prefilled `value`, the LangGraph edit case):

```json
{ "kind": "form", "message": "Review the reply before I send it.",
  "dataSchema": { "type": "object",
    "properties": {
      "subject": { "type": "string" },
      "body": { "type": "string", "x-ui": { "widget": "textarea" } } },
    "required": ["subject", "body"] },
  "value": { "subject": "Re: invoice", "body": "Thanks, received." },
  "actions": ["submit", "respond", "cancel"] }
```

The form opens prefilled; `submit` returns the edited content. Response: `{ "action": "submit", "content": { "subject": "Re: invoice #42", "body": "Thanks, received and approved." } }`.

**Tool-call approval** (a permission request, the most common case):

```json
{ "kind": "form", "intent": "tool_permission", "message": "Approve this database write?",
  "dataSchema": { "type": "object",
    "properties": { "table": { "type": "string" }, "rows": { "type": "integer", "minimum": 1 } },
    "required": ["table", "rows"] },
  "value": { "table": "invoices", "rows": 4 },
  "actions": ["submit", "decline", "cancel"] }
```

The proposed call is prefilled; `submit` approves (edited values become modified arguments), `decline` denies. The platform records the outcome tagged with `intent` for metrics. See [Tool-permission and approval workflows](#tool-permission-and-approval-workflows).

## Migration

No action for existing agents or deployments. Text chat keeps working as-is, and no existing agent's behavior changes, since `ask_user` is opt-in rather than injected. Agents gain elicitation through first-class adapters (Mastra/LangGraph/MCP) or by calling `render()`/`elicit()`. Web chat eligibility stays gated on the `web` adapter, and no new spec field enables elicitation.

## Milestones

1. **Spine:** `renderable.proto`, bridge `sendRenderable` + live-await correlation + `resume` entry point, SSE event + idempotent response endpoint (sidecar `/api/chat/...`, server-side validation), sidecar SQLite interactions table + store methods.
2. **Web rendering:** JSON Forms renderer, `waiting-for-input` gating via the streaming-state mechanism, queue head in composer, reload re-render.
3. **Happy-path resume:** deliver responses over the open stream with indefinite waiting; the SDK already auto-reconnects, and optional gRPC keepalive (server + client) trims reconnection latency on long-idle streams.
4. **Adapter mappings:** Mastra first (native JSON Schema serialization + clean suspend/resume makes it the cleanest proof), then Claude Agent SDK (`PreToolUse` tool-permission → `tool_permission` elicitation), LangGraph, AI SDK + provided opt-in `ask_user` helper.
5. **Tool-permission workflow + telemetry:** the `tool_permission` intent, approval-card rendering, and structured outcome emission to the OTel → Langfuse pipeline.
6. **Recovery:** the sidecar delivers answered-but-undelivered responses from SQLite on the agent's reconnect; the framework `resume` entry point applies them, wired through Mastra and LangGraph checkpointers; verify an answer after an agent restart resumes.
7. **Slack rendering (inline blocks + button→modal):** single-field interactions inline, multi-field forms via the modal handshake, authorized-responder + implicit-cancel handling; the cross-platform proof that the portable *data* model holds where layout doesn't.
8. **(Later) Component strategy:** MCP Apps-compatible sandboxed rendering on web.

## Open Questions

- JSON Forms renderer vs a thin custom renderer for the web client; either way it renders the data schema plus `x-ui` hints, and the wire model is unaffected.
- Go JSON Schema 2020-12 validator choice, and keeping its semantics in parity with the client-side JS validator.
- Surface the `DECLINE` vs `CANCEL` distinction in v1 UI, or collapse them (they fold to the same LangGraph `ignore`).
- Slack degradation contract for arrays, arrays-of-objects, and deep nesting beyond flat primitives.
- Confirm the sidecar chat volume is a **persistent volume** (not emptyDir), per astropods/messaging#61; the durability model assumes the store survives a pod reschedule. (The PR text is currently inconsistent on this.)
- Cross-restart recovery for non-checkpointed (live-session) agents via conversation replay, deferred; confirm the v1 boundary (form survives reload; an in-flight live await is abandoned on restart, response preserved) is acceptable.
- Which frameworks beyond Claude Agent SDK (`PreToolUse`), Mastra (`requireApproval`), and LangGraph (interrupt-before-tool) expose a tool-approval hook the adapter can map to `tool_permission`.
- Teams and Discord rendering specifics (atomic multi-field submit, widget set, option caps) before those adapters are targeted; the Slack guidance here is researched, those are not yet.
