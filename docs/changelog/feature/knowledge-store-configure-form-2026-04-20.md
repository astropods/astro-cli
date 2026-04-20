# Knowledge Stores Configure Form

## Summary

Follow-up polish pass on the Knowledge Stores experience — configure form, list page header, empty state, copy, and assets. Builds on the Table primitives and provider picker introduced in PR #710. No behavior changes.

## Design

**Configure form.** Mirrors the deploy form's layout and field conventions: `FormSection` composites for section headers, `Label size="md"` for fields, `space-y-12` between sections and `space-y-5` between fields. The submit row matches the deploy form's `border-border mt-12` separator, ghost Cancel, and button sizing. Mode radio cards and the PrivateLink card use `InterfacesPicker`-style token set — `border-primary/40 bg-primary/5` selected, `border-border bg-transparent` default — and the page shell switches from `bg-muted` to `bg-surface` so transparent card fills read correctly. The PrivateLink card owns the whole external-connection group (toggle, host/port inputs, skip-connection-test checkbox), mirroring the deploy-page "Web + Require authentication" nesting pattern. Skip-connection-test uses a native checkbox with `accent-primary`, consistent with the onboarding terms checkbox.

**UX writing.** Em dashes and jargon ("backbone", "reachability") removed from the configure form. "Make private," "PrivateLink," and "Skip connection test" descriptions rewritten with front-loaded action + consequence framing. "Skip health check" renamed to "Skip connection test" to match what the toggle actually does. Storage options trimmed to friendly labels (10 GB / 20 GB / 50 GB / 100 GB / 1 TB); backend values stay as Kubernetes quantities.

**Select primitive.** Fixed `SelectTrigger` placeholder color — the `placeholder:` Tailwind variant never matched Radix's trigger, causing placeholders to render in foreground. Switched to `data-[placeholder]`, which Radix exposes, so every Select in the app now renders its placeholder in `text-muted-foreground` like `Input` does.

**List page header.** Scope switcher removed; the page now scopes to the active account via `useActiveAccount`. Header buttons stack below the description on narrow viewports (`flex-col` on mobile, `flex-row sm:` up).

**Empty state.** Aligned to the Agents page empty state pattern: dashed border upgraded to `border-strong`, icon moved into a `size-12 rounded-md bg-border` container so it reads against the `bg-muted` page background, and spacing updated to match (`mb-4` icon, `mb-2` heading, `mb-6` body with `max-w-sm`).

**Table.** Column labels sentence-cased to match the Table primitive introduced in PR #710. Status-cell pattern (no dot indicator, sentence-cased values) applied to the knowledge stores list.

**Top nav.** Renamed "Knowledge" → "Knowledge Stores" to match the page heading.

**Integration icons.** Replaced the placeholder Pinecone glyph with the official cone and swapped in the official MySQL dolphin mark, across light and dark variants.

## Migration

No migration required.
