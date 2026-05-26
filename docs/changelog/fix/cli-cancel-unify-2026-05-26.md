# Uniform abort paths on push visibility prompt + dim Cancelled. in `ast configure`

## Summary

Two reviewer non-blockers on #1147 (the prior cancel-UX sweep). Both abort paths on the `ast push` visibility prompt now collapse onto the canonical sentinel, and `ast configure`'s TUI abort prints the same dim `Cancelled.` line every other surface uses.

## Design

**Push visibility prompt** — the prompt has two abort paths (esc/ctrl+c, and an explicit "No") that previously diverged:

- esc/ctrl+c returned raw `tui.ErrCancelled`, which cobra surfaced as an exit-1 failure with the bare `cancelled` message.
- "No" returned `fmt.Errorf("push cancelled")`, also exit-1, with a different message.

The pipeline step in `cmd/pipeline.go` now returns `tui.ErrCancelled` for both. `runPush` (`cmd/push.go`) catches it with `errors.Is(err, tui.ErrCancelled)` and prints the dim `Cancelled.` line via `printCancelled(os.Stdout)`, returning `nil` for a clean exit-0 — same pattern `secrets`, `account`, `repair`, and `login` already follow.

**`ast configure`** — `runConfigure`'s TUI-abort branch was still calling `fmt.Println("Cancelled.")`, missing the shared dim styling. Replaced with `printCancelled(cmd.OutOrStdout())`.

`internal/tui/add` was also flagged by the reviewer for not following the shared `tui.ErrCancelled` pattern, but its only caller (`cmd/add.go`) is `//go:build ignore`-d dead code slated for removal; deferred until that command's future shape is decided.

## Migration

No action required.
