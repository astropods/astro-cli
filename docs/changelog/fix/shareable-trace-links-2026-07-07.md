# feat: share a link to a trace

## Summary

Users had no way to hand someone a link to a specific trace in the observability view. The trace panel was already URL-addressable, but that was invisible: there was no button to grab the link, and the selected time window was not encoded, so a shared link lost the surrounding context. Closes #1183.

## Design

The trace detail panel already syncs the open trace into the URL (`?trace=<id>`) and hydrates a trace even when it falls outside the currently loaded window, so a deep link already reopens the right trace. This change makes that shareable:

- A "Copy link" button (the Lucide `link` icon) sits next to the trace id in the panel header, alongside a "Copy trace id" button; both are 12px icon buttons. Copy link copies the current URL and confirms with a "Link copied" toast, or a "Couldn't copy link" error toast if the clipboard write fails. It reuses the existing `useCopyToClipboard` hook, matching the established "Copy deploy ID" pattern. On the right, a light divider separates the prev/next trace navigation from the expand/close panel controls.
- The monitor time window (7D / 14D / 30D) is now stored in the URL (`?window=<range>`) instead of local component state, so a copied link reopens with the same window selection. This preserves the relative range (for example, the last 7 days), not the exact absolute interval the sender was viewing, so a link opened later resolves that range relative to when it is opened. A missing or unknown value falls back to the default (7D).

## Migration

None. This is a new capability; existing links and behavior are unchanged.
