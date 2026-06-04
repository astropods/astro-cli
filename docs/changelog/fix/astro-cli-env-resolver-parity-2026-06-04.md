# Unify astro-cli and astro-server credential env-var names on the resolver

## Summary

For any given `astropods.yml`, the env-var names `ast dev` injected into the
local container could differ from what `astro-server` injected on deploy.
Agents that worked locally could break on first push (and vice versa). The
divergence had four sources, all stemming from each caller re-deriving §8.1
credential names instead of asking the spec package.

## Design

`packages/astro-spec` is the only place credential env-var names are
derived. Two public entry points cover all current callers:

- `CloudCredentialKeys(spec)` — built-in cloud providers
  (`anthropic`/`openai`/…) on `models` / `knowledge` / `integrations`.
- `CustomProviderCredentialKeys(spec)` — user-defined `providers:` entries.

Both implement the §8.1 rule (`<PROVIDER>_[<NAME>_]<SUFFIX>`, with bare
qualification only when entry name matches provider) including duplicate
handling and `SanitizeEnvName` for hyphenated names.

### Sites unified

1. **`apps/astro-cli/internal/compose/builder.go`** — replaced the three
   per-section blocks (models / knowledge / integrations) using
   `strings.ToUpper(name) + "_" + cs.Suffix` with one `CloudCredentialKeys`
   loop. Replaced the custom-provider block (which emitted both
   `CLOUDFLARE_ACCOUNT_ID` and the bare `ACCOUNT_ID` "for convenience")
   with `CustomProviderCredentialKeys`. The CLI reads only the
   resolver-correct prefixed name from `.env` — no bare-suffix fallback,
   so a misnamed `.env` fails fast in dev instead of silently working in
   dev and breaking in prod.

2. **`apps/astro-server/internal/deployment/validator.go`** —
   `GetRequiredCredentials` had its own §8.1 implementation using
   `SanitizeName` (keeps hyphens) instead of `SanitizeEnvName`
   (underscores), and emitted a bare key even when no entry name matched
   the provider. Now delegates to `spec.CloudCredentialKeys` + filters
   managed providers (server supplies their credentials, so users
   shouldn't be asked).

3. **`apps/astro-cli/cmd/add.go`** — `builtinCredentials` computed names
   locally. Build-tag-ignored today but un-ignoring would have re-introduced
   drift. Now parses the post-add spec and asks `CloudCredentialKeys`.

### Structural guard

`packages/astro-spec/envresolver_parity_test.go` walks
`apps/astro-server` and `apps/astro-cli` for the banned `+ cs.Suffix` /
`+ <anything>.Suffix` patterns and fails CI if either app reintroduces
local derivation. Test files are exempt (legitimate assertions against
the canonical resolver). Skipped outside a repo checkout.

## Migration

**Breaking for agents that read the dev-only `<NAME>_<SUFFIX>`
convention** in their `.env` or agent code. The env-var name `ast dev`
injects now matches what the deployer injects in prod — there is one
naming convention across local and prod, not two.

| Spec | Old (`ast dev` only) | New (`ast dev` + prod) |
|---|---|---|
| `models.claude.provider: anthropic` | `CLAUDE_API_KEY` | `ANTHROPIC_API_KEY` |
| `models.gpt.provider: openai` | `GPT_API_KEY` | `OPENAI_API_KEY` |
| `models.alpha.provider: anthropic` + `models.beta.provider: anthropic` | `ALPHA_API_KEY` / `BETA_API_KEY` | `ANTHROPIC_ALPHA_API_KEY` / `ANTHROPIC_BETA_API_KEY` |

Run `ast push` and inspect the deployer's required-credentials list to
see exactly which names your spec expects.

For custom providers, drop any reads of the bare variable name
(`ACCOUNT_ID`) and use the prefixed form (`CLOUDFLARE_ACCOUNT_ID`) — and
rename the entry in `.env` to match. The CLI no longer falls back to the
bare suffix; a missing prefixed key surfaces as a missing-credential
error on `ast dev` startup.
