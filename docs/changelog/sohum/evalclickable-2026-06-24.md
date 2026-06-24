## Summary

Eval review queue traces can now open the full trace detail panel from the trace header itself. This keeps the review controls focused on verdicts while still giving reviewers a quick path into the complete trace.

## Design

The eval dataset page reuses the same trace detail panel used by monitoring, including the right-side slide-in behavior and responsive overlay fallback. The review queue converts the selected trace summary into the trace entry shape expected by the panel, so the panel can render the full input/output context without duplicating monitor-specific UI.

The trace header no longer uses a separate right-side action button. Instead, the trace id is part of an always-visible `View trace_xxxxxx` affordance with a trailing arrow; hover and keyboard focus shift the color to make the action feel clickable without changing the layout. Opening the panel disables the affordance until the panel closes, which keeps one active trace detail surface on the page.

## Migration

No user action is required.
