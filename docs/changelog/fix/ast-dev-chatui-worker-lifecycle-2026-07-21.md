# Summary

Fixes the local chat-UI worker (`chatui-serve`) that `ast dev` / `ast project`
launch for the embedded chat interface. It could be left holding the fixed port
`127.0.0.1:3100` across runs, and even a clean run printed a spurious
"Chat UI port ... is held by another process" warning.

# Design

Three issues in the CLI's chat-UI worker handling, all in `cmd/chatui.go`
(plus the `dev.go` call site):

- **False "port held" warning.** `startChatUI` read `proc.Process.Pid` *after*
  `proc.Process.Release()`, which invalidates it, so the readiness probe compared
  the health endpoint's pid against an invalid value and reported the port busy on
  every run, even when the worker had bound successfully. It now captures the pid
  before releasing the process handle.
- **Orphan reclaim.** The worker is a detached session leader on a fixed, shared
  port, so one leaked by a force-quit (or started for another agent, since every
  agent uses the same port) can't be displaced via the per-project pid file.
  Startup now finds whatever `chatui-serve` actually holds the port (by port, via
  `lsof`) and terminates it, escalating to SIGKILL, before rebinding. A
  non-chatui listener is left untouched.
- **Exit with the launching CLI.** In foreground/`--local` the worker is started
  with `--exit-with-parent` and watches for reparenting, so it exits when the CLI
  dies by any means (double Ctrl+C, kill), instead of orphaning and squatting the
  port. Background mode omits the flag so the worker still outlives the exiting CLI.

# Migration

None. Local-dev only; no change to deployed agents.
