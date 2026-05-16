# Expand pod detail panel to full width

## Summary

On the deployment details page, the pod detail panel was fixed at 42rem on the right side. For pods with long environment-variable values, dense event lists, or wide log lines, that width forced wrapping and truncation even when the rest of the page had plenty of room. The panel needed a user-controlled way to take the full viewport width without forcing every user into the narrow-viewport overlay mode.

## Design

The pod panel's full-width state is owned by `AgentDeployments` (alongside the existing selected-pod and overlay-threshold state), not by the panel itself, because the parent already coordinates the graph translate. A new `podPanelExpanded` flag collapses three previously distinct concerns into one predicate, `podPanelFullWidth = panelOpen && (podPanelExpanded || shouldOverlay)`:

- panel chrome — side-mode (`w-[42rem]` anchored bottom-right) vs. full-width (`inset-3 top-20`)
- graph translate — only shifts when the panel sits beside the graph, i.e. `panelOpen && !shouldOverlay && !podPanelExpanded`
- effective graph width — same condition; expanded/overlay returns the full container width so the graph re-centers underneath

`expanded` / `onToggleExpanded` are passed down to `PodDetailPanel`, which renders a Maximize2/Minimize2 button next to the close button. The toggle is hidden when `shouldOverlay` is already true — the panel is full-width in that mode regardless, so the control would be a no-op. The flag resets on pod close and on pod switch so the next selection starts in the default side-mode.

## Migration

None.
