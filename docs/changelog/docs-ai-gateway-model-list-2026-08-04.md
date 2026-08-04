# AI Gateway docs list the full served model set

## Summary

The AI Gateway serves eleven models, but the public docs described nine. The
supported-models table was missing `qwen3-coder` and `qwen3-vl`, so agents had
no way to discover two models they were already entitled to call. The table now
matches the gateway's alias map in `modules/astro-infra/helm/values/common/bifrost.yaml.tpl`,
which is the source of truth for what a tenant key can address.

## Design

The served set is defined once, as the `aliases` map on the Bedrock provider plus
the matching allowlist on the key. Public surfaces are projections of that map,
so the docs were reconciled against it rather than against other docs:

- Added the two Qwen rows to the supported-models table.
- `qwen3-vl` accepts image input, so the vision section and the image-generation
  caveat now name both it and `pixtral-large` instead of `pixtral-large` alone.
- The quick-start spec example advertised `gpt-4o` in a `provider: gateway` model
  menu. The gateway does not serve it, so a deployer picking it would get a 401
  at runtime. It now uses `claude-haiku-4-5`.

`us.anthropic.claude-sonnet-4-6` is deliberately absent. It is not an alias, and
the Sonnet entry resolves to a `global.` inference profile rather than a `us.`
one, so documenting it would point users at an id the key's allowlist rejects.

The pricing page carries its own copy of the model catalog and had drifted the
same way. It lives in the `modules/website` submodule
(`postman-eng/astro-ai-website`), so it is fixed in a companion PR there rather
than here; the submodule pointer bump follows once that PR merges.

## Migration

None. No spec keys, env vars, or model ids changed, and the gateway's served set
is unchanged. Agents could always call `qwen3-coder` and `qwen3-vl`; only the
documentation was behind.
