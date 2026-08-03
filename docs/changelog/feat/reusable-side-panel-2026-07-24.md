# Reusable side-panel shell

## Summary

The right-side detail and inspector panels (traces, datasets, pods, chat) each rolled their own shell: the same container chrome, a header row with expand/close controls, and a scrollable body, duplicated per panel with small drifts. This adds a shared `SidePanel` shell so the chrome and controls live in one place, and migrates all three real side panels onto it: the trace detail panel (used by both the traces and datasets views), the pod detail panel, and the chat inspector.

## Design

`SidePanel` (`components/ui/side-panel.tsx`) owns the container chrome (`border`, `bg-surface`/`bg-card`, rounded, `overflow-hidden`, `role="dialog"`) and a header row that renders a `title` slot on the left and, on the right, a `headerActions` slot followed by the built-in expand and close buttons. The body is the caller's `children`, so each panel keeps its own layout and scroll regions.

Panel content stays layout-only and feeds the shell:

```tsx
<SidePanel ariaLabel="Trace details" onClose={...} onToggleExpanded={...} expanded={...}
  title={<TracePanelTitle ... />} headerActions={<TraceNavButtons ... />}>
  {/* meta grid, tabs, scrollable body */}
</SidePanel>
```

The shell runs in two modes so a panel keeps its current look where it genuinely differs, rather than being forced into one mold:

- Inline mode (traces, pods): the shell draws the chrome and header. `headerBorder` (default on) drops the header divider for panels whose header flows straight into tabs (pods).
- Docked mode (chat): setting `open` hands the shell ownership of the open/close transition. On desktop it slides the panel in and out (the width and translate/opacity animation); on small screens it presents the same content as a bottom `Sheet`. It also owns the two-phase mount, so the content and its queries stay idle while the panel is closed. The docked content brings its own header, so ChatWorkspace no longer wires up an `<aside>`, a `Sheet`, or the mount/enter state; it just renders `<SidePanel open={...}>`.

```tsx
// docked (chat): the shell owns the animation and the mobile sheet
<SidePanel open={inspectorOpen} onClose={...} ariaLabel="Agent details">
  <ChatInspectorPanel ... />
</SidePanel>
```

Per-panel notes:

- Traces: `TracePanelHeader` split into `TracePanelTitle` (timestamp + trace id + copy/share) and `TraceNavButtons` (prev/next), fed into the shell's `title`/`headerActions`; the shell owns the expand and close controls it used to render itself.
- Pods: header (name + status badge) feeds `title`, tabs and per-tab bodies stay as `children`, `headerBorder={false}` preserves the divider-less header.
- Chat: `ChatInspectorPanel` is layout-only content; the docked `SidePanel` around it owns the frame, the width animation, and the mobile sheet, so `SidePanel` is now the single side panel component across the app.

## Migration

None. All three panels render and behave identically; the shell is presentation-only.
