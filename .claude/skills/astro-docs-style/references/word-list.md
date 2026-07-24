# Astro docs word list

The authoritative term reference for checking Astro docs copy. Prose-level
conventions (voice, structure, components) live in `docs-public/AGENTS.md`; this
file is the mechanical "avoid / prefer" reference. Keep it small and high-signal
— add entries when a real ambiguity comes up, not preemptively.

## Astro brand terms — correct form

| Wrong / ambiguous | Correct | Notes |
|---|---|---|
| Astro Pods, astropods (prose) | **Astropods** | One word, capital A. The platform, registry, and spec. `astropods.yml` (lowercase, backticked) is the spec file. |
| Astro, Astro.ai | **Astro AI** | Full product name (site title is "Astro AI"). "Astro" alone is fine after first use. |
| `Ast`, AST, the Ast CLI | the **`ast`** CLI | Command name is lowercase and backticked. "the `ast` CLI", `ast blueprint push`. |
| Blueprint (mid-sentence noun) | **blueprint** | Lowercase common noun: "push a blueprint". Capitalize only to start a sentence or in a heading. |
| Agent (mid-sentence noun) | **agent** | Lowercase. An agent is a live, running instance of a blueprint. |
| AI gateway, ai-gateway (prose) | **AI Gateway** | Capital A, capital G. `ai-gateway` is fine as a slug/path. |
| Knowledge Store, knowledgestore | **knowledge store(s)** | Lowercase, two words. |
| Agent Card (the file) | **agent card** (concept) / `AGENT.md` (file) | Lowercase for the concept; the file itself is `AGENT.md`. |
| Account (mid-sentence noun) | **account** | Lowercase common noun. |
| secret reference forms | `KEY=@SECRET_NAME` | Reference a vault secret with `@` + secret name at deploy time. |

Product/section names that keep their casing: **Astropods Spec**, **Agent Card
Spec**, **Messaging SDK**, **Adapters**, **AI SDK**, **Mastra**, **LangChain**,
**Claude Agent SDK**.

## Avoid / prefer — general terms

| Avoid | Prefer | Notes |
|---|---|---|
| abort | end, stop, cancel | |
| utilize, leverage | use | |
| allow / enable / let (to describe what the product makes possible) | rephrase around the user action | "Deploy an agent to…", not "Astro lets you deploy…". |
| in order to | to | |
| so that | so, to | |
| simply, just, easily | (delete) | Hedges that add nothing. |
| make sure you, you'll want to | rephrase as a requirement | "X requires Y". |
| via | with, through, using | |
| e.g. / i.e. (in prose) | for example / that is | Fine inside parentheses. |
| and/or | or (usually) | |
| kill (a process) | stop, end | |
| whitelist / blacklist | allowlist / blocklist | |
| master (branch/node) | main / primary | |
| sanity check | quick check, confirm | |

## Capitalization and casing

| Rule | Example |
|---|---|
| Sentence case for titles, headings, card titles (except RFC/spec pages `astropods-package-spec`, `agent-card-spec`, which may keep Title-Case numbered headings) | "Deploy your first agent", not "Deploy Your First Agent" |
| `AI`, `API`, `CLI`, `URL`, `YAML`, `JSON`, `SDK`, `MCP`, `OAuth`, `IP`, `SQLite` | all-caps (OAuth and SQLite as shown) |
| CLI commands, flags, file names, env vars | backticked: `ast agent list`, `--json`, `astropods.yml`, `ANTHROPIC_API_KEY` |
| Model providers | `anthropic` (as a CLI value, lowercase/backticked); "Anthropic" (the company, prose) |

## US spelling

Use US spelling: **canceled/canceling** (one l), **behavior**, **color**,
**catalog**, **license** (noun and verb), **initialize/-ization**. Avoid British
variants (cancelled, behaviour, colour, catalogue, licence, initialise).

## Formatting quick rules

- Inline code (commands, flags, file names, keys, values): single backticks.
- UI labels and buttons: **bold**.
- Click-paths: bold each segment and the separator — **Settings** > **Secrets**.
- Code fences take a language and optional title: ` ```yaml title="astropods.yml" `.
- Don't wrap a bare URL in a sentence — link a noun phrase instead.
