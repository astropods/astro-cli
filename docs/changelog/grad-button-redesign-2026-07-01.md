# Review queue detail — header consolidation

## Summary

The review queue detail view had a separate bottom footer holding the verdict buttons, which competed for vertical space with the trace content. This consolidates all chrome into the header so the content area gets the full remaining height.

## Design

The detail view is now header + content, with no footer:

- The header is a vertical stack. Row one keeps the trace link, sentiment badge, and `X / Y` position indicator (`justify-between`). Row two holds the verdict controls (buttons + error message) on their own full-width line, so nothing overlaps at narrow container widths.
- The verdict controls component (formerly `ReviewQueueVerdictFooter`, now `ReviewQueueVerdictControls`) keeps its keyboard shortcut handling and button refs; only its footer chrome (top border, full-bar padding) was dropped. The `G / B / N` shortcut hint text was removed — the shortcut chips on the buttons already convey it — while the save-error message is retained.
- Resizable content sections now default to their minimum height and scroll internally, rather than expanding to fit content. Users still drag the corner handle (or arrow-key) to grow a section.

## Migration

None required.
