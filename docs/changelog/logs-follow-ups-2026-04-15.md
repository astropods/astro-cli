## Summary

Follow-up polish on the log viewer UI and live-tail behavior introduced in the previous log streaming work. The changes address toolbar layout at different viewport sizes, tailing state visibility, and correctness of the auto-scroll behavior.

## Design

**Toolbar layout.** The toolbar is split into two groups: filters (errors/warnings) on the left alongside the `leading` slot, and controls (search, time range, tail toggle, copy) on the right. In compact mode the search bar expands to fill available width; in normal mode it has a fixed 240px width with right-alignment. Compact mode replaces filter text labels with icons to save space. The toolbar wraps cleanly when horizontal space is tight.

**Live tail button.** The tail toggle now shows a Play/Pause icon that reflects current state, plus a contextual tooltip ("Start live tailing" / "Stop live tailing"). The old ambient pulsing dot was moved out of the button into the log area itself — a blinking teal dot and status line appear at the top of the log stream while tailing is active, and in the empty-state message while waiting for the first line.

**Auto-scroll fix.** The original single `useEffect` ran the tailing-start reset (`isUserScrolled = false`, `setShowJumpToBottom(false)`) on every dependency change, including every incoming log line while tailing. This was split into two effects: one keyed on `isTailing` that resets scroll state and jumps to bottom when tailing starts, and one keyed on `logs.length` that scrolls on new log entries. This prevents spurious state updates in the scroll hot path.

**Dropdown font.** The container selector dropdown switched from `font-mono` to `font-sans` to match the rest of the UI.

## Migration

No action required.
