# Renderable Schema Specification

| | |
|---|---|
| **Version** | 0.8 (Draft) |
| **Status** | Draft, for review |
| **Date** | 2026-06-29 |
| **Companion** | [Interactive Rendering & Elicitation Specification](interactive-rendering-elicitation-spec.md) |

## Abstract

A `Renderable` of `kind: form` carries a **data schema** (JSON Schema 2020-12) with optional inline **render hints** (`x-ui`). This specification defines the supported subset of JSON Schema, the render-hint vocabulary, the two conformance profiles that bound what a surface must render, and the validation rules applied to a response. The companion spec defines the `Renderable` envelope, transport, lifecycle, and security; this document defines only schema content, rendering, and validation.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

"JSON Schema" means the 2020-12 dialect. "JSON Pointer" means [RFC 6901](https://www.rfc-editor.org/rfc/rfc6901). A "surface" is a rendering target (web client, Slack adapter, future targets). A "consumer" is the code that renders a Renderable and produces a response.

---

## 1. Scope

A form Renderable carries one document, the data schema, transmitted as a JSON string:

| Field | Source | Description |
|-------|--------|-------------|
| `data_schema_json` | `Renderable.data_schema_json` | JSON Schema 2020-12. Defines fields, validates the `SUBMIT` response `content`, and drives rendering. REQUIRED. |

Presentation is expressed by optional inline **render hints** (`x-ui`) on individual properties (§4); they are advisory and honored per surface. Properties render in declaration order. There is no separate UI/layout document in v1; expansive layout (groups, tabs, conditional visibility) is deferred to the component render strategy.

A consumer MUST ignore any keyword it does not recognize rather than reject the schema. Keywords outside this specification carry no guaranteed behavior.

This document does NOT define the `Renderable` envelope, `allowed_actions`, transport, persistence, or resumption. See the companion spec.

---

## 2. Conformance Profiles

### 2.1 Core Profile

A data schema is **Core** if it conforms to §3.1 through §3.8 and uses no Extended construct (§3.9). A surface MUST render every Core schema. The Core profile is identical to MCP elicitation's `requestedSchema` restriction; every MCP elicitation is a valid Core schema.

### 2.2 Extended Profile

A data schema is **Extended** if it uses any construct in §3.9 (nesting, arrays of objects, `oneOf`/`anyOf` of object schemas, conditionals). A surface MAY render Extended schemas. A titled `oneOf` of `const` values (§3.6) is a single-select enum and stays Core.

### 2.3 Surface Tiers and Degradation

A surface declares its supported tier through `AdapterCapabilities` (companion spec). A surface that receives an Extended schema it cannot render MUST follow the companion spec's degradation and failure contract: for a free-text-tolerant ask, render the prompt as text and return `RESPOND`; for a strict ask, return a typed `UNSUPPORTED` failure. A surface MUST NOT silently drop a Renderable.

Core renderability is also bounded by §2.4; a surface that cannot render a Core schema for capability reasons degrades the same way.

### 2.4 Surface Capability Limits

Core membership bounds schema *complexity* (flat primitives and enums), not a surface's rendering *capacity*. A consumer MAY be unable to render a Core schema for reasons orthogonal to the profile:

- **Missing widget.** A surface without a date picker or checkbox renders the affected field with the nearest available widget or degrades it (§3.3, §3.5).
- **Option cardinality.** A surface MAY cap single/multi-select options (some clients cap at 25). Beyond the cap the consumer degrades (overflow, search, or text). Authors SHOULD keep option counts small.
- **No atomic multi-field submit.** A surface whose messages cannot submit several fields at once MUST gather a multi-field form through a surface that can (for example a modal opened on a user action) or degrade. Single-field interactions need no atomic submit.

In every case the consumer degrades or refuses per §2.3 and MUST NOT silently drop fields.

---

## 3. Data Schema

### 3.1 Root Object

| Constraint | Requirement |
|------------|-------------|
| Type | The root MUST be `{"type": "object"}`. |
| `properties` | REQUIRED. A map of property name to property schema (§3.2–§3.9). |
| `required` | OPTIONAL. Array of property names that MUST be present in `content`. |
| `additionalProperties` | OPTIONAL. Defaults to `false` (a deliberate profile override: standard JSON Schema treats an absent value as permissive, so both validators in §5 MUST apply the `false` default explicitly). When `false`, `content` with keys not in `properties` MUST fail validation. |

The `SUBMIT` response `content` is a JSON object keyed by property name. A single-value prompt is still a one-property object.

### 3.2 String

A property MAY be `{"type": "string"}`. Supported keywords:

| Keyword | Type | Description |
|---------|------|-------------|
| `title` | string | Field label. Defaults to the property name. |
| `description` | string | Helper text. |
| `default` | string | Prefilled value (§3.10). |
| `minLength` | integer | Minimum length. Enforced. |
| `maxLength` | integer | Maximum length. Enforced. |
| `pattern` | string | ECMA-262 regular expression the value MUST match. Enforced. |
| `format` | string | One of §3.3. |
| `enum` | string[] | Restricts to listed values; the property becomes a single-select (§3.6). |

Default widget: single-line text input.

### 3.3 String Formats

A `format` value MUST be one of the following. A consumer encountering an unknown `format`, or lacking the matching widget (a date picker), MUST render the property as a plain string or degrade per §2.4.

| `format` | Widget | Client validation |
|----------|--------|-------------------|
| `email` | email input | RFC 5322 addr-spec |
| `uri` | URL input | absolute URI |
| `date` | date picker | RFC 3339 full-date |
| `date-time` | date-time picker | RFC 3339 date-time |

### 3.4 Number and Integer

A property MAY be `{"type": "number"}` or `{"type": "integer"}`. Supported keywords:

| Keyword | Type | Description |
|---------|------|-------------|
| `title`, `description` | string | Label, helper text. |
| `default` | number | Prefilled value. |
| `minimum`, `maximum` | number | Inclusive bounds. Enforced. |
| `exclusiveMinimum`, `exclusiveMaximum` | number | Exclusive bounds. Enforced. |
| `multipleOf` | number > 0 | Value MUST be a multiple. Enforced. |
| `enum` | number[] | Single-select over numeric values (§3.6). |

Default widget: number input. `integer` rejects non-integral values.

### 3.5 Boolean

A property MAY be `{"type": "boolean"}`. Supported keywords: `title`, `description`, `default` (boolean). Default widget: checkbox or switch. A surface without one renders it as a two-option select or a confirm/deny button pair.

### 3.6 Enumerations (Single-Select)

A property with `enum` restricts the value to a fixed set and renders as a single-select. Two equivalent forms:

| Form | Shape | Labels |
|------|-------|--------|
| `enum` + `enumNames` | `{"type":"string","enum":["a","b"],"enumNames":["A","B"]}` | `enumNames` is positional to `enum`; OPTIONAL. |
| Titled `oneOf` | `{"oneOf":[{"const":"a","title":"A"},{"const":"b","title":"B"}]}` | `title` per option. Preferred when labels are REQUIRED (MCP SEP-1330). |

The response value is the chosen `const`/`enum` member. Default widget: a dropdown; request `radio` or `buttons` via `x-ui` (§4). A surface MAY cap the option count (§2.4); authors SHOULD keep enums small.

### 3.7 Multi-Select

A property MAY be an array whose `items` is an enumeration:

```json
{ "type": "array", "items": { "type": "string", "enum": ["a", "b", "c"] } }
```

| Keyword | Type | Description |
|---------|------|-------------|
| `minItems`, `maxItems` | integer | Selection count bounds. Enforced. |
| `uniqueItems` | boolean | When `true`, duplicates fail validation. |

The response value is an array of chosen members. Default widget: checkbox set or multi-select. Option-count caps apply as in §3.6. Multi-select is the only array form in the Core profile; arrays of objects are Extended (§3.9).

### 3.8 Sensitive data (not supported)

Elicitation MUST NOT be used to collect sensitive data (passwords, API keys, secrets); responses are persisted and may transit logs or traces. v1 has no secret-field handling: `format: "password"` and `writeOnly` carry no special platform behavior and are not part of the supported set. See the main spec's Security section.

### 3.9 Extended Types

The following are valid only on Extended surfaces (§2.2). On a Core-only surface they MUST degrade (§2.3).

| Construct | Shape | Renders as |
|-----------|-------|------------|
| Nested object | a property with `"type": "object"` | labeled group |
| Array of objects | `"items"` is an object schema | repeatable list with add/remove |
| Variant | `oneOf` / `anyOf` of object schemas | variant selector; `oneOf` SHOULD use a shared discriminator property |
| Conditional | `if` / `then` / `else`, `dependentRequired`, `dependentSchemas` | conditional fields |

### 3.10 Defaults and Prefilled Values

| Source | Meaning | Precedence |
|--------|---------|------------|
| `default` (per property) | schema-level prefill | lower |
| `Renderable.value` (top-level) | agent-proposed current value for edit-in-place | higher |

When both are present for a property, `Renderable.value` MUST win.

---

## 4. Render Hints (`x-ui`)

Presentation is advisory. A property MAY carry an `x-ui` object; a consumer honors it where the surface and the property's type support it, otherwise it falls back to the type's default widget. `x-ui` never affects validation (§5), and an unknown or incompatible hint is ignored, never an error.

```json
{ "type": "string", "enum": ["a", "b", "c"], "x-ui": { "widget": "radio" } }
```

`x-ui` fields:

| Field | Type | Description |
|-------|------|-------------|
| `widget` | string | Requested widget; see §4.1. |

### 4.1 Widget × type mapping

A `widget` is honored only on a compatible property type. On an incompatible type it is ignored and the type's default widget (§4.2) is used.

| `widget` | Valid on | Behavior |
|----------|----------|----------|
| `textarea` | `string` without `enum` | Multi-line text input. |
| `radio` | single-select `enum` (string or number) | Radio group (one choice). |
| `select` | single-select `enum`; multi-select array | Dropdown (single) or multi-select dropdown (array). Also the default for an enum. |
| `buttons` | single-select `enum`; `boolean` | One button per `enum` member, a click submits it (see §4.3); a boolean renders as a yes/no pair. |
| `slider` | `number`/`integer` with both `minimum` and `maximum` | Slider across the bounds. Ignored if either bound is absent. |

### 4.2 Default widgets

When a property has no `x-ui` (or an incompatible one), the type determines the widget:

| Property | Default widget |
|----------|----------------|
| `string` | single-line text input |
| `string` with `format` (§3.3) | the format's input or picker |
| `string`/`number` with `enum` | dropdown (`select`) |
| multi-select array of `enum` | checkbox set |
| `number`/`integer` | number input |
| `boolean` | checkbox / switch |

### 4.3 Rendering Conventions

- **One-tap selection.** When the root has exactly one REQUIRED enum property and `allowed_actions` contains `SUBMIT`, a consumer MAY render the options as direct buttons where a click submits that value (the `buttons` widget makes this explicit). Adding `RESPOND` adds an "or type your own" affordance.
- **Actions are not data.** `SUBMIT`, `DECLINE`, `CANCEL`, and `RESPOND` come from `allowed_actions` and render as the form's action controls. Enum members are data-schema values. A consumer MUST NOT model preset options as actions.

---

## 5. Validation

| Stage | Requirement |
|-------|-------------|
| Client | The consumer MUST validate against the data schema before enabling `SUBMIT`. On failure it MUST keep the form open and show inline errors. |
| Sidecar | The messaging sidecar MUST validate `SUBMIT` `content` against the stored data schema before delivering it to the agent. On failure the response endpoint MUST reject and the interaction MUST remain `pending`. (astro-server only proxies; see the companion spec.) |

Enforced keywords: `required`, `type`, `enum`, `minLength`, `maxLength`, `pattern`, `minimum`, `maximum`, `exclusiveMinimum`, `exclusiveMaximum`, `multipleOf`, `minItems`, `maxItems`, `uniqueItems`, `additionalProperties`, and `format` where checkable.

Additional rules:

- A `RESPOND` response carries free-text `text` and MUST NOT be validated against the data schema.
- A `SUBMIT` response `content` MUST always be validated.
- Client and server validators MUST use the JSON Schema 2020-12 dialect and SHOULD produce equivalent verdicts.
- On a surface without a held, atomically-submitted form (inline chat components), client-side pre-validation is limited to natively supported checks (length, number bounds); the remaining constraints are enforced server-side on submission, and the interaction stays `pending` on failure. A modal MAY return per-field errors (for example Slack `response_action: errors`).

---

## 6. Wire Encoding

`data_schema_json`, `value_json`, and the response `content_json` are JSON strings within the proto messages. The messaging sidecar MUST parse `data_schema_json`, reject malformed JSON rather than emit it, and re-embed it as a JSON object in the SSE `interaction` event. `content` is validated (§5) before it is delivered to the agent.
