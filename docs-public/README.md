# Astro AI Docs

Public product documentation built with [Fern](https://buildwithfern.com).

## Contents

- **Get started** — Install the CLI, get started with `ast`, authentication
- **API reference** — Astro AI REST API (agents, register, push, config) generated from OpenAPI

## Local development

Requirements: Node 18+, [Fern CLI](https://buildwithfern.com/learn/cli-api-reference/cli-reference/overview) (`npm install -g fern-api`).

```bash
cd fern
fern docs dev
```

Open http://localhost:3000 to preview.

## Project layout

- `fern/docs.yml` — Navigation, theme, instance URL
- `fern/docs/pages/` — MDX pages (welcome, install-cli, get-started, authentication, api-reference-overview)
- `fern/apis/astro-api/openapi.yaml` — OpenAPI 3.1 spec for the Astro AI API (inferred from astro-server)
- `fern/apis/astro-api/generators.yml` — Fern generator config for the API reference

## Publishing

```bash
cd fern
fern generate --docs
```

Configure the instance URL and organization in `fern/docs.yml` and `fern/fern.config.json`.
