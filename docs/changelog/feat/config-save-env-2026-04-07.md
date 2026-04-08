## Summary

Adds `ast config --save <file>` to export stored project config vars to a `.env`-style file. This is the inverse of the existing migration flow (which imports from `.env` into the config store) — useful for sharing configs with tools that expect a `.env` file, seeding CI secrets, or creating a backup.

## Design

A `--save <file>` flag is added to the `configure`/`config` command. When provided, the command skips the interactive form entirely and reads the project's stored vars from `~/.ast/project-configs.json`, then writes them to the specified file using `godotenv.Write`, which produces standard `KEY=VALUE` dotenv format.

```sh
ast config --save .env
ast config --save secrets.env
```

The command reports the number of vars exported and the destination path. If no stored vars exist for the current project, it prints an informational message and exits cleanly.

## Migration

No changes required. Existing `configure` behavior is unchanged; `--save` is an opt-in flag.
