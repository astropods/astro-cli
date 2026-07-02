## Summary

First-time chat visitors with at least one deployed agent didn't know they could switch which agent they're chatting with. A one-time coachmark now points at the agent switcher in the chat header ("Switch agents here"), with a subtle indigo highlight on the switcher itself. It dismisses permanently once the user opens the switcher or closes the coachmark.

## Design

- New `ChatAgentSwitchCoachmark` component: a small bubble anchored under the switcher with a pointer notch, a swap icon, a close button, and a gentle bob animation. Entrance and bob keyframes live in `index.css` and respect `prefers-reduced-motion`.
- `ChatThreadHeader` gates the coachmark on a one-shot `astro:chat-agent-switch-coachmark-seen` flag in localStorage, read into local state on mount. Because the header only renders when a chat-eligible agent is scoped, the coachmark is inherently limited to users who have an agent to chat with.
- While the coachmark is visible, the switcher gets a subtle inset indigo ring plus a low-opacity fill (semantic `primary` tokens, brightened to `primary-400` in dark mode for contrast). The ring is drawn inset to avoid clipping against the header's `overflow-hidden` bounds.
- `AgentDeploymentMenu` gained an `onOpenChange` passthrough so opening the switcher dismisses the coachmark.
- The coachmark wrapper is a polite live region (`role="status"`, `aria-live="polite"`) so screen readers announce the nudge when it appears.

## Migration

No action required.
