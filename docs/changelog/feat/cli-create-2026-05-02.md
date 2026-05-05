## Summary

`ast project create` has been redesigned from an interactive TUI wizard into a non-interactive command. `ast project start` now runs in the foreground by default. All remaining `ast dev` references in user-facing output have been replaced with `ast project` equivalents.

## Design

**`ast project create` — non-interactive harness generator**

The `huh`-based wizard (name, description, model provider prompts) has been removed. Name is now a required positional argument validated as kebab-case. The default scaffold description is replaced with a placeholder that instructs the user to fill it in.

`--model` accepts `anthropic`, `openai`, or `ollama[/<model>]` (e.g. `ollama/llama3.3:70b`). Tab completion for `--model` is context-aware: typing `ollama/` enumerates known models; after the `:` only the tag suffix is returned to avoid shell word-break duplication.

After generating the agent harness, the CLI presents a `huh` TUI input asking what the agent should do, then prints a platform context prompt to paste into Claude or another coding agent. Ctrl+C or Esc during input skips the question and prints the prompt anyway. Pass `-y` / `--yes` to skip non-interactively. The prompt includes the platform description, the agent name and goal, and an instruction for the coding agent to ask what the user wants.

`--model anthropic` (or `openai`) no longer emits a duplicate model entry in `astropods.yml` when the provider was already present in the default config.

`ast spec explain` is now hidden. Error messages say "project name" instead of "blueprint name".

**`ast project start` — foreground by default**

The default behavior is now to tail all container logs in the foreground and auto-stop on Ctrl+C. The ready-block footer shows "Ctrl+C to stop". The previous detached behavior is available via `-b` / `--background`, which exits immediately and shows `ast project logs` / `ast project stop` hints instead.

**Message cleanup**

All `ast dev` references in error messages and status output replaced with `ast project` equivalents (`ast project start`, `ast project logs`, `ast project trigger <name>`, `ast project start --local`).

**huh theme consistency**

All `huh` TUI forms (configure inputs, delete-dotenv confirm, create goal input) now use the CLI's primary color for focused titles via a shared `cliHuhTheme()` helper.

## Migration

- `ast project create` now requires the agent name as a positional argument.
- `-y` / `--yes` on `create` skips the goal prompt (it no longer skips a TUI wizard).
- `ast project start` now runs in the foreground. Scripts or CI jobs that relied on the command returning immediately should add `-b` / `--background`.
