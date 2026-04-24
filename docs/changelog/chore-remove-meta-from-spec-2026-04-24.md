## Summary

The `meta` block in `astropods.yml` (containing `visibility`) was never consumed by the platform — visibility is stored in and managed via the database, set through the CLI (`ast blueprint create --visibility`, `ast blueprint update --visibility`) or the platform UI. The spec field was dead code. This change removes it from the required fields and stops emitting it in scaffolded specs.

## Design

`meta` is made optional rather than removed entirely to preserve backward compatibility: existing specs with a `meta:` block still parse and validate successfully (the schema has `additionalProperties: false`, so removing the property definition entirely would break them). The `Meta` struct stays in place with `omitempty` tags; the schema no longer lists it in `required`.

The scaffold generator (`generateAstropodsYml`) no longer emits the `meta:` block, so newly created agents won't have it. The public spec doc is updated to mark `meta` and all its fields (`description`, `tags`, `visibility`) as deprecated.

## Testing

- Push an existing `astropods.yml` that includes a `meta: visibility: private` block — it should parse and register without errors.
- Scaffold a new agent and confirm the generated `astropods.yml` contains no `meta:` block.
- Verify `ast blueprint create --visibility public` and `ast blueprint update --visibility private` still set visibility correctly.

## Migration

Nothing required. Existing `astropods.yml` files with `meta:` continue to work. Visibility is set via `ast blueprint create --visibility` or `ast blueprint update --visibility`.
