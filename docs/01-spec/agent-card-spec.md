# Agent Card Spec

**Version:** 1.0
**Date:** 2026-03-09
**Status:** Draft

## Abstract

An agent card is a Markdown file with YAML frontmatter that serves as the public-facing documentation for an agent. It is purely descriptive — it communicates what an agent is and what it can do but does not drive any functional behavior (deployment, visibility, etc.), which remains the responsibility of `astropods.yml`.

Analogous to HuggingFace's model card, the agent card lives in the agent's repository as `AGENT.md` and is submitted alongside the spec during registration.

## Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

---

## 1. File Format

An agent card is a UTF-8 encoded Markdown file named `AGENT.md` at the root of the agent's project directory (same level as `astropods.yml`).

The file consists of two parts:

1. **Frontmatter** — A YAML block delimited by `---` lines at the top of the file. Contains structured metadata for discovery and attribution.
2. **Body** — Free-form Markdown following the frontmatter. Contains human-readable documentation: what the agent does, how to use it, limitations, examples, etc.

Both parts are OPTIONAL. A valid agent card MAY be an empty file, a file with only frontmatter, or a file with only a Markdown body (no `---` delimiters).

---

## 2. Frontmatter Schema

All frontmatter fields are OPTIONAL.

### 2.1 `authors`

A list of people or organizations who built the agent.

Each entry MUST contain a `name` field and MAY contain an `account` field that references a platform account handle. When `account` is present, the client SHOULD render it as a link to that account's profile page.

```yaml
---
authors:
  - name: Jane Doe
    account: janedoe
  - name: Acme Corp
    account: acme
  - name: External Contributor
---
```

| Field     | Type   | Required     | Description |
|-----------|--------|--------------|-------------|
| `name`    | string | **REQUIRED** | Display name of the author. |
| `account` | string | OPTIONAL     | Platform account handle. When present, clients link to `/{account}`. |

### 2.2 `capabilities`

A list of high-level capabilities the agent provides. These are short, human-readable phrases that describe what the agent can do. They serve as discovery hints and are displayed on the agent's detail page.

Unlike tags (which are broad categories), capabilities describe specific behaviors.

```yaml
---
capabilities:
  - Analyzes GitHub issues and extracts patterns
  - Builds and maintains a Neo4j knowledge graph
  - Answers natural-language queries about issue history
---
```

Each entry MUST be a string. Entries SHOULD be concise (under 100 characters) and written as verb phrases describing an action the agent performs.

### 2.3 `integrations`

A list of third-party services or platforms the agent connects to. Used to display brand logos and connection requirements on the agent's detail page.

Each entry is a string. When the string matches a **known integration** (see Section 2.3.1), the client renders the corresponding brand icon. Unknown strings are displayed with a generic icon and the raw name as a label.

```yaml
---
integrations:
  - Slack
  - GitHub
  - Jira
  - My Custom API
---
```

Matching is **case-insensitive** — `slack`, `Slack`, and `SLACK` all resolve to the same known integration.

#### 2.3.1 Known Integrations

The platform maintains a registry of known integrations with brand icons. This registry is defined in `packages/astro-spec/agent_card_integrations.json` and serves as the canonical list.

Each entry in the registry contains:

| Field  | Type   | Description |
|--------|--------|-------------|
| `id`   | string | Canonical lowercase identifier used for matching. |
| `name` | string | Display name rendered in the UI. |

The initial registry:

| ID | Display Name |
|----|-------------|
| `slack` | Slack |
| `github` | GitHub |
| `linear` | Linear |
| `notion` | Notion |
| `google-drive` | Google Drive |
| `gmail` | Gmail |
| `jira` | Jira |
| `confluence` | Confluence |
| `discord` | Discord |
| `microsoft-teams` | Microsoft Teams |
| `salesforce` | Salesforce |
| `zendesk` | Zendesk |
| `twilio` | Twilio |
| `stripe` | Stripe |
| `shopify` | Shopify |
| `asana` | Asana |
| `trello` | Trello |
| `figma` | Figma |
| `dropbox` | Dropbox |
| `airtable` | Airtable |

New integrations are added by appending to the JSON registry and adding a corresponding icon asset to the client. The registry is intentionally broader than the current client icon set — entries without a client icon yet fall back to the generic icon until one is added.

#### 2.3.2 Matching Rules

To resolve an agent card integration string to a known integration:

1. Normalize the input: lowercase, trim whitespace.
2. Look up by exact match against known `id` values.
3. If no match, look up by exact match against known `name` values (lowercased).
4. If no match, treat as an unknown integration — display with generic icon and the original string as the label.

This allows authors to write either `github` or `GitHub` and get the same result, while also accepting arbitrary strings like `My Custom API` gracefully.

---

## 3. Body

The body is free-form GitHub-Flavored Markdown (GFM). There is no required structure, but authors are encouraged to cover:

- **Overview** — What the agent does and why it exists.
- **Usage** — How to interact with the agent (APIs, protocols, chat, etc.).
- **Limitations** — Known constraints, failure modes, or scope boundaries.
- **Examples** — Sample interactions or outputs.

---

## 4. Parsing

The `astro-spec` package MUST provide a parser that accepts the raw content of an `AGENT.md` file and returns:

1. A structured representation of the frontmatter (with zero-value defaults for absent fields).
2. The body as a raw Markdown string.

The parser MUST tolerate:
- Missing frontmatter (no `---` delimiters) — returns empty metadata and the full content as body.
- Empty frontmatter (`---\n---`) — returns empty metadata and the remaining content as body.
- Unknown frontmatter fields — ignores them without error.
- Missing file — returns empty metadata and empty body without error.

The parser MUST reject:
- Malformed YAML in the frontmatter block — returns a parse error.

---

## 5. Registration Flow

During `astro push`, the CLI:

1. Reads `AGENT.md` from the project root (if present).
2. Parses frontmatter and body using the spec parser.
3. Submits the full raw content of `AGENT.md` as the `readme` field in the registration payload (replacing the current readme mechanism).

The server stores the raw agent card content in `agent_versions.readme`. The client parses frontmatter on read to extract structured metadata for display.

> **Migration:** Existing agents without an `AGENT.md` continue to work. The `readme` field remains empty or contains any previously submitted content. No breaking changes to the registration API are required — the field name and storage remain the same.

---

## 6. Client Display

The client renders the agent card on the agent detail page:

- **Frontmatter metadata** is displayed in structured UI elements (author links, capability chips/badges, integration icons).
- **Integrations** are rendered as brand icons (known) or generic icons with labels (unknown), using the existing `IntegrationIconStack` component pattern.
- **Body** is rendered as styled Markdown using the existing `StyledMarkdown` component.
- **Description** continues to come from `astropods.yml` `meta.description` and is displayed separately (sidebar, cards, SEO).

---

## 7. Example

```markdown
---
authors:
  - name: Jane Doe
    account: janedoe
  - name: Bob Smith
capabilities:
  - Analyzes GitHub issues and extracts patterns
  - Builds and maintains a Neo4j knowledge graph
  - Answers natural-language queries about issue history
integrations:
  - GitHub
  - Slack
  - My Custom Webhook
---

# GitHub Issue Analyzer

This agent ingests GitHub issues into a Neo4j knowledge graph and provides
natural-language querying over the resulting data.

## How It Works

1. Connect your GitHub repository via the `GITHUB_TOKEN` input.
2. The agent polls for new issues on a configurable schedule.
3. Each issue is analyzed using an LLM to extract entities and relationships.
4. Results are stored in Neo4j and queryable via the agent's chat interface.

## Limitations

- Only processes issue bodies and comments; PR reviews are not yet supported.
- Large repositories (>10k issues) may require extended initial ingestion time.
```
