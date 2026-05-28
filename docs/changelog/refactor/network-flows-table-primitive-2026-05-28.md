# NetworkFlowsTable uses the shared Table primitive

## Summary

The destinations/routes/databases table on the agent monitor page was rolling its own table markup — raw `<table>`/`<thead>`/`<tbody>` with bespoke padding, hover, and borders — while the rest of the app uses the `Table` primitive from `@/components/ui/table`. This refactor moves it onto the primitive so it inherits shared chrome and stays consistent with sibling tables (top spenders, audit log, knowledge stores).

## Design

### Primitive adoption

`NetworkFlowsTable` now composes `Table` / `TableHeader` / `TableBody` / `TableFooter` / `TableHead` / `TableRow` / `TableCell`. Loading and empty states render as single full-width `TableCell`s with `colSpan` rather than as siblings outside the table. The "Show N more / Show less" toggle moved into a `TableFooter` row so it sits inside the same bordered container.

The separate "Destinations / N peers" strip above the table is gone — the count now sits inline next to the column header (`Destination 12`), matching the `TopSpendersTable` pattern.

### Container-level styling override

To match the surrounding `NetworkSummaryCard` tiles on the monitor page, this instance needs `bg-card dark:bg-surface` background and `rounded-lg` corners — but the primitive's `className` prop only forwards to the `<table>` element, not its outer container.

The `Table` primitive gained a `containerClassName` prop that merges into the outer container's classes via `cn`:

```tsx
<Table
  className="min-w-[600px] bg-card dark:bg-surface"
  containerClassName="rounded-lg"
>
```

Existing consumers continue to render with the default `rounded-sm border border-border` chrome — the new prop is purely additive.

### Test fix

`AgentMonitor.test.tsx` had a racy assertion (`findByText("Requests")` followed by a synchronous `getByText(/0 total requests/)`). The old custom table only mounted its header when data existed, so `findByText("Requests")` happened to resolve late enough that the chart subtitle had also rendered. The refactored table renders its `Requests` column header immediately, exposing the race. The test now awaits the subtitle text directly via `waitFor`, matching the pattern already used by its sibling test.

## Migration

No changes required for consumers of the `Table` primitive — `containerClassName` is optional and defaults to no override.
