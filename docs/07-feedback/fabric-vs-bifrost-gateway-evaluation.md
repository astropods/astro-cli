# LLM Gateway Evaluation for Astro: Fabric, Bifrost, and LiteLLM

**Gateways evaluated:** Postman Fabric, Bifrost, LiteLLM
**Recommendation:** Bifrost Gateway
**In short:** Fabric has notable feature gaps for our platform use case (detailed below). LiteLLM and Bifrost are effectively feature-equivalent, so the choice between those two came down to performance, where Bifrost was substantially lighter and faster.

---

## Executive Summary

Astro requires an LLM gateway it can operate as infrastructure: the Astro team administers the gateway, while Astro's end users consume it. This is a platform use case, not a SaaS use case. We evaluated three gateways against this requirement — **Postman Fabric**, **Bifrost**, and **LiteLLM**.

**Fabric** is currently designed as a SaaS product for direct human users, and this orientation surfaces as six gaps for a platform deployment: no operator/customer domain separation, no infrastructure-as-code support, no separation between admin and consumer API planes, no opaque standard-protocol proxy, an incomplete virtual key implementation, and no model input/output price tracking or pricing plugins for monetization. These are detailed in the sections below. Until Fabric closes them, we don't think it's ready to serve as Astro's gateway.

**Bifrost and LiteLLM** both address these requirements — on features, the two are effectively equivalent. The deciding factor between them was runtime performance and resource footprint. In our testing, LiteLLM consumed roughly **4 GB of RAM at idle (no load)** and showed about **6 s of latency**, while Bifrost held to roughly **500 MB of RAM** and about **11 µs of latency**. At platform scale that difference is large enough to be decisive, so Bifrost is the recommendation. The sections below document the feature bar that Fabric misses and that both Bifrost and LiteLLM clear.

---

## 1. SaaS Design vs Platform Design

### 1.1 No operator/customer domain separation

Fabric models users, groups, and organizations as a single flat identity layer. There is no distinction between **gateway operators** (the team running the gateway) and **gateway consumers** (the customers whose traffic flows through it).

For Astro, this distinction is fundamental: the Astro team are the *users* of Fabric, but the consumers of the gateway are *Astro's users*. Fabric's user/group model does not translate to customers — there is no entity in the data model that represents a downstream customer of the gateway operator.

Bifrost does not have this problem. **Customers are a first-class data model**, separate from the users who administer the gateway ([Bifrost: Create Customer](https://docs.getbifrost.ai/api-reference/governance/create-customer)). This enables per-customer governance, attribution, and lifecycle management without overloading the identity system.

### 1.2 No infrastructure-as-code path

Fabric's current documentation provides no IaC mechanism for configuring the control plane. Standing up Fabric means bringing up infrastructure and then completing configuration manually through the UI. This is a serious limitation:

- The control plane cannot be reproducibly provisioned, versioned, or reviewed.
- Coding agents and CI pipelines cannot set up or manage the gateway — manual steps break automation entirely.
- Environment parity (dev/staging/prod) becomes a manual, drift-prone process.

Bifrost supports full declarative configuration with `config.json` as source of truth ([Bifrost: Source of Truth](https://docs.getbifrost.ai/deployment-guides/config-json/source-of-truth)), and Terraform can configure the entire gateway alongside the underlying infrastructure. The whole stack is codified end to end.

**On its own, this is close to disqualifying for us:** any component Astro adopts must be provisionable by automation.

---

## 2. API Design Is a Poor Fit for Programmatic Consumption

### 2.1 Melded auth planes

A gateway serving a platform needs two distinct API surfaces:

1. **Admin/governance APIs** — used by the operator (Astro) to manage customers, keys, budgets, routing, and policy.
2. **Consumer gateway APIs** — the inference proxy used by end users.

In Fabric, these are melded into the same auth plane. There is no way to issue operator credentials that are structurally distinct from consumer credentials, which makes programmatic gateway management by Astro's backend both awkward and risky (over-privileged tokens, no blast-radius isolation).

### 2.2 Bifrost's separation

Bifrost cleanly separates the two surfaces, allowing deployment as:

- `admin.gateway.astropods.ai` — governance/admin APIs, operator auth
- `gateway.astropods.ai` — the inference proxy, consumer auth

Distinct hosts, distinct auth, distinct threat models. This is arguably **the most consequential gap for our use case** — without it, Astro can't cleanly manage the gateway programmatically with appropriately scoped credentials.

---

## 3. No Standard Opaque Proxy Endpoints

Consumers of the gateway need an **opaque proxy** speaking a standard API surface — OpenAI-compatible, Anthropic-compatible, or Gemini-compatible. Provider selection and backend routing must be resolved internally by the gateway, invisible to the caller.

Fabric instead exposes routing through explicit routes that the consumer must target. This pushes routing concerns onto the consumer:

- End users must know about gateway-internal topology.
- The operator cannot change provider backends, failover, or load distribution without consumer-visible changes.
- Standard SDKs (OpenAI/Anthropic/Gemini clients pointed at a base URL) cannot be dropped in cleanly.

Route-based addressing is useful for sandbox testing, but it isn't a good fit as the consumer interface for Astro's users. Routing should be configuration, not API surface.

Bifrost implements exactly this model: it exposes 100%-compatible protocol endpoints (`/openai`, `/anthropic`, `/genai`) so existing SDKs work with only a base URL change, while provider credentials, routing, load balancing, and fallbacks are resolved inside the gateway ([Bifrost: Drop-in Replacement](https://docs.getbifrost.ai/features/drop-in-replacement), [OpenAI SDK integration](https://docs.getbifrost.ai/integrations/openai-sdk/overview), [Google GenAI integration](https://docs.getbifrost.ai/integrations/genai-sdk/overview)).

---

## 4. Gateway Keys Fall Short of Virtual Keys

### 4.1 Gateway Keys lack required semantics

Fabric exposes "Gateway Keys," which superficially resemble virtual keys but lack the features that make virtual keys useful:

- **No rate/usage limits** per key
- **No budgets** per key
- **No customer mapping** — keys cannot be bound to a customer entity (which, per §1.1, doesn't exist)

Bifrost's virtual keys are the primary governance entity and carry all of this natively: per-key budgets (rolling or calendar-aligned), request/token rate limits, model and provider allowlists, and exclusive attachment to a team or customer. Consumers authenticate with the virtual key using standard headers — `Authorization: Bearer` (OpenAI style), `x-api-key` (Anthropic style), or `x-goog-api-key` (Gemini style) — so the key model composes with the opaque proxy from §3 ([Bifrost: Virtual Keys](https://docs.getbifrost.ai/features/governance/virtual-keys)). Budgets cascade hierarchically (Customer → Team → Virtual Key → Provider), with one request debiting every applicable level ([Bifrost: Governance](https://www.getmaxim.ai/bifrost/resources/governance)).

### 4.2 Unclear credential model

The coexistence of "Gateway Credentials" and "Service Credentials" in the same place has no rationale that's clear to us. It isn't obvious what each is for, when to use which, or how they compose — which suggests the credential model may not have been designed against a concrete multi-tenant use case.

### 4.3 Observability consequence

Without proper virtual keys bound to customers, **customer identity cannot be propagated into OTel traces**. Per-customer attribution of latency, cost, errors, and usage — table stakes for operating a gateway on behalf of customers — is not achievable.

---

## 5. No Model Input/Output Price Tracking

Astro needs to charge its users for gateway consumption. That requires the gateway to meter usage at the token level and price it:

- **Per-request cost attribution** — input tokens, output tokens (and cache read/write where applicable), priced against a per-model price table.
- **Pricing configuration** — operator-managed price tables per model/provider, with the ability to apply margin (cost price vs sell price) so Astro can bill above provider cost.
- **Aggregation by customer** — usage and cost rolled up per customer (which again depends on the customer entity and virtual key mapping from §1.1/§4), exportable to a billing system.
- **Plugin/extension hooks** — a plugin surface where custom pricing, metering, or billing-export logic can run in the request path (e.g., emit priced usage events to Astro's billing pipeline).

Fabric currently provides none of this — no price tables, no cost computation on requests, and no plugin mechanism to add it. Without gateway-level price tracking, Astro would have to reconstruct cost from raw token counts (if even exposed per customer) in a separate pipeline, re-deriving provider pricing out of band and keeping it in sync manually.

Bifrost covers this: it computes request cost against a built-in pricing catalog that auto-syncs from a remote datasheet, and **Custom Pricing** lets the operator override rates at runtime — scoped globally, per provider, per provider key, or per virtual key, with wildcard model matching and per-request-type filtering ([Bifrost: Custom Pricing](https://docs.getbifrost.ai/providers/custom-pricing)). Virtual-key-scoped overrides are the margin mechanism: catalog price = Astro's cost, VK-scoped override = customer's sell price. Computed costs debit the hierarchical budgets from §4, giving per-customer priced usage for billing export. For custom metering/billing logic in the request path, Bifrost supports Go/WASM plugins ([Bifrost: Custom Plugins](https://docs.getbifrost.ai/enterprise/custom-plugins) — note this is positioned under the enterprise tier).

This compounds the §4.3 observability gap: cost is the single most important per-customer metric for a monetized gateway, and Fabric can neither compute it nor attribute it.

---

## Conclusion

| Requirement | Fabric | Bifrost |
|---|---|---|
| Customer as first-class entity, separate from operators | ✗ Flat user/group/org model | ✓ [Customer data model](https://docs.getbifrost.ai/api-reference/governance/create-customer) |
| IaC / Terraform provisioning of control plane | ✗ Manual configuration required | ✓ [config.json source of truth](https://docs.getbifrost.ai/deployment-guides/config-json/source-of-truth) + Terraform |
| Separate admin vs consumer API/auth planes | ✗ Single melded auth plane | ✓ [Governance API](https://docs.getbifrost.ai/api-reference/governance/create-customer) (admin) vs [proxy endpoints](https://docs.getbifrost.ai/features/drop-in-replacement) (consumer) |
| Opaque standard proxy (OpenAI/Anthropic/Gemini) | ✗ Consumer-visible route-based addressing | ✓ [/openai, /anthropic, /genai drop-in endpoints](https://docs.getbifrost.ai/features/drop-in-replacement) |
| Virtual keys with limits, budgets, customer mapping | ✗ Gateway Keys lack all three | ✓ [Virtual keys](https://docs.getbifrost.ai/features/governance/virtual-keys) with budgets, rate limits, customer attachment |
| Customer-attributed OTel traces | ✗ Blocked by key model | ✓ Via virtual keys |
| Token-level price tracking + pricing plugins for billing | ✗ No price tables, cost computation, or plugin hooks | ✓ [Custom pricing](https://docs.getbifrost.ai/providers/custom-pricing) + [plugins](https://docs.getbifrost.ai/enterprise/custom-plugins) |

Fabric is built as a SaaS product for direct users; Astro needs a platform component for operating a gateway on behalf of its own customers. Each of the gaps above traces back to that mismatch. Until Fabric provides (1) a customer domain model, (2) IaC-driven control plane setup, (3) split admin/consumer auth planes, (4) opaque standard-protocol proxying, (5) real virtual keys, and (6) model input/output price tracking with plugin hooks for billing, **it isn't a fit for Astro at this time**. Bifrost Gateway meets these requirements today and is the recommended path — and next to the feature-equivalent LiteLLM, it is far lighter and faster at runtime.
