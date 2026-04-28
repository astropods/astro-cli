## Summary

Reorganizes local-development commands under `ast project`, consolidates `ast configure` flags, and adds a `--model` shortcut to `ast project create` so the LLM provider can be set without going through the interactive form.

## Design

### `ast dev` → `ast project`

`ast dev` is renamed to `ast project` (the old name is kept as an alias for backward compatibility). Subcommands follow the same rename:

```
ast project create   # was: ast create (top-level alias still works)
ast project start    # was: ast dev start / ast dev
ast project logs     # was: ast dev logs
ast project stop     # was: ast dev stop
ast project trigger  # was: ast dev trigger
ast project configure  # was: ast configure
```

`create` and `configure` move from the root level into `ast project`. Top-level `ast create` continues to work as an alias.

### `--model` flag on `project create`

```
ast project create my-agent --model anthropic
ast project create my-agent --model openai
ast project create my-agent --model ollama
ast project create my-agent --model ollama/llama3.3:70b
```

Pre-selects the LLM provider and skips the interactive model-selection step. `--yes` and `--model` can be combined for a fully non-interactive create. `--model ollama/<model>` validates against the known Ollama model list.

### `ast project configure` flag consolidation

`configure set` and `configure unset` subcommands are removed; their functionality is now expressed as flags on `configure` itself:

| Old | New |
|---|---|
| `ast configure set KEY VALUE` | `ast project configure --var KEY=VALUE` |
| `ast configure unset KEY` | `ast project configure --rm-var KEY` |
| `ast configure` (interactive) | `ast project configure` (interactive, unchanged) |
| `ast configure --out env\|json` | `ast project configure --out env\|json` |

A new `--vars-file` flag imports variables from an env file in one shot:
```
ast project configure --vars-file .env.local
```

`configure telemetry` is removed; it was already superseded by `ast settings update --telemetry on|off`.

### `blueprint validate` removed

`ast blueprint validate` / `ast validate` are removed; they were already superseded by `ast spec validate`.

### `ast configure` top-level alias

`ast configure` is now a hidden top-level alias for `ast project configure`, so existing scripts and muscle-memory continue to work without going through `ast project`.

### `ast secrets import` requires `--file`

The positional argument is replaced with an explicit flag:

```
ast secrets import --file .env
ast secrets import -f .env.local
```

This makes the intent unambiguous and aligns with the `-f` convention used by other spec-reading commands.

### `blueprint push` builds by default; `--no-build` to skip

`ast blueprint push` (and `ast push`) now build the container image automatically before pushing. Pass `--no-build` to skip the build step if the image is already up to date:

```
ast blueprint push my-agent            # build + push
ast blueprint push my-agent --no-build # push only
```

### `blueprint build` and `blueprint push` name is optional

The `<name>` argument is now optional on both commands. When omitted, the name is read from `astropods.yml`:

```
ast blueprint build            # name from spec
ast blueprint push             # name from spec
ast blueprint push my-agent    # name overrides spec
```

## Migration

| Old command | New command |
|---|---|
| `ast dev [start]` | `ast project start` |
| `ast dev logs` | `ast project logs` |
| `ast dev stop` | `ast project stop` |
| `ast configure set KEY VALUE` | `ast project configure --var KEY=VALUE` |
| `ast configure unset KEY` | `ast project configure --rm-var KEY` |
| `ast configure telemetry` | `ast settings update --telemetry on\|off` |
| `ast blueprint validate` / `ast validate` | `ast spec validate` |
| `ast secrets import <file>` | `ast secrets import --file <file>` |
| `ast blueprint push my-agent --build` | `ast blueprint push my-agent` (build is now default) |
| `ast blueprint push my-agent` (push only) | `ast blueprint push my-agent --no-build` |

`ast dev`, `ast create`, and `ast configure` continue to work unchanged; the old names are aliases.
