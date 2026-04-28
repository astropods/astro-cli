## Summary

Adds a new `ast settings` command group that consolidates telemetry control and shell completion generation, replacing scattered flags on `ast configure`.

## Design

```
ast settings update --telemetry on|off   # enable or disable anonymous telemetry
ast settings bash                        # write bash completion script
ast settings zsh                         # write zsh completion script
ast settings fish                        # write fish completion script
ast settings powershell                  # write PowerShell completion script
```

Completion scripts are written to `~/.ast/<binary>-completion.<shell>` instead of stdout. After writing, the command prints the file path and shell-specific sourcing instructions (e.g. `source ~/.ast/ast-completion.bash`).

Telemetry control (`--telemetry`, `--no-telemetry`, `configure telemetry`) is removed from `ast configure` and consolidated into `ast settings update --telemetry on|off`.

## Migration

- `ast configure --telemetry` / `ast configure telemetry --enable/--disable` → `ast settings update --telemetry on|off`
- Shell completion output now writes to a file; source the printed path instead of redirecting stdout.
