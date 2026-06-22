# Fix: repo selector opens on full-control click

## Summary

On the blueprint create flow, the GitHub repository selector only opened its
dropdown when the user clicked the small chevron icon. Clicking the rest of the
control merely focused the hidden search input without revealing the repo list,
making the affordance feel broken.

## Design

The repo selector is a single composite control: a search input, a clear
button, and a chevron toggle, all wrapped in one container. Previously only the
chevron's handler set `repoOpen`; the container click handler just focused the
input. The fix moves the open behavior up to the container so any click on the
control body opens the dropdown and focuses the input. The chevron retains its
toggle (open/close) behavior via `stopPropagation`, and keyboard focus and the
outside-click-to-close logic are unchanged, preserving accessibility. When a
repo is already selected the control shows the selection and its clear button
instead, so the open-on-click behavior is suppressed in that state.

## Migration

None. Behavior-only fix.
