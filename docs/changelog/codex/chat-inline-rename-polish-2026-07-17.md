# Summary

Move chat-specific interaction polish out of the surface token update and into a focused chat experience change. The chat header, title rename flow, composer actions, and details inspector have their own behavior and review surface, so they are easier to evaluate separately from global color-token tuning.

# Design

The chat title becomes the rename affordance directly in the chat header. It saves on blur or Enter, cancels on Escape, and keeps the title width tied to the rendered text so editing does not cause the header layout to jump. Rename handling is removed from the chat history dropdown so that history remains a lightweight navigation and delete surface.

The chat composer, user message bubble, and rendered message links use shared button and semantic text/surface styles instead of custom round icon-button hover states, low-contrast dark fills, or accent-colored markdown links. The details inspector keeps its structural layout while using semantic card and muted surfaces for the panel, active state, system prompt, and tool sections.

# Migration

No user action is required. Existing conversations continue to use the same persisted title field; the rename entry point moves from chat history to the active chat title.
