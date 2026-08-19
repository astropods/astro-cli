import { Fragment, useMemo, useState } from "react";
import { ChevronDown, ChevronRight, Info, X } from "lucide-react";
import { Card } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  type SortDirection,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { IdentityLabel } from "@/components/activity/TopSpendersTable";
import { categoryColor } from "@/components/activity/model-colors";
import { formatCompact, formatCost, formatDateShort } from "@/lib/format-utils";
import { cn } from "@/lib/utils";
import type { InsightsSourceLabel, InsightsSourcePeople } from "@/lib/api";

/** The category a reader drilled into. */
export interface SourceFilter {
  axisKey: string;
  axisLabel: string;
  labelKey: string;
  labelName: string;
  colorIndex: number;
}

// "axis:<key>" sorts by the label currently measured in that axis's column.
type SortKey = "share" | "count" | "cost" | "total" | `axis:${string}`;

// Naming a label turns its column into a sortable measure; TOP_CATEGORY is the
// at-a-glance default.
const TOP_CATEGORY = "__top__";

function labelFor(labels: InsightsSourceLabel[] | undefined, key: string) {
  return labels?.find((l) => l.key === key);
}

/** Answers "who is behind this category". */
export function PeopleTable({
  people,
  filter,
  onClearFilter,
  day,
  onClearDay,
  loading,
  axes,
}: {
  people: InsightsSourcePeople;
  filter: SourceFilter | null;
  onClearFilter: () => void;
  day: string | null;
  onClearDay: () => void;
  loading: boolean;
  axes: { key: string; label: string }[];
}) {
  const [sortKey, setSortKey] = useState<SortKey | null>(null);
  const [descending, setDescending] = useState(true);
  const [openKey, setOpenKey] = useState<string | null>(null);
  const [measures, setMeasures] = useState<Record<string, string>>({});
  const optionsByAxis = useMemo(
    () => Object.fromEntries(axes.map((a) => [a.key, axisLabelOptions(people.rows, a.key)])),
    [axes, people.rows],
  );
  const measureFor = (axisKey: string) => measures[axisKey] ?? TOP_CATEGORY;

  // Sort follows the question on screen.
  const activeSort: SortKey = sortKey ?? (filter ? "share" : "total");

  const rows = useMemo(() => {
    const scored = people.rows.map((row) => {
      const hit = filter ? labelFor(row.axes[filter.axisKey], filter.labelKey) : undefined;
      return {
        row,
        count: hit?.traces ?? 0,
        share: hit?.traces_pct ?? 0,
        cost: hit?.cost_usd ?? row.cost_usd,
      };
    });
    const visible = filter ? scored.filter((s) => s.count > 0) : scored;
    const axisKey = activeSort.startsWith("axis:") ? activeSort.slice("axis:".length) : "";
    const measure = axisKey ? measures[axisKey] : "";
    const by = (s: (typeof scored)[number]) => {
      if (axisKey) {
        if (!measure || measure === TOP_CATEGORY) return s.row.traces;
        return labelFor(s.row.axes[axisKey], measure)?.traces_pct ?? 0;
      }
      return activeSort === "share" ? s.share
        : activeSort === "count" ? s.count
        : activeSort === "cost" ? s.cost
        : s.row.traces;
    };
    return visible
      .map((s) => ({ s, k: by(s) }))
      .sort((a, b) => {
        const delta = a.k - b.k;
        if (delta !== 0) return descending ? -delta : delta;
        return a.s.row.key.localeCompare(b.s.row.key);
      })
      .map(({ s }) => s);
  }, [people.rows, filter, activeSort, descending, measures]);

  function sortBy(key: SortKey) {
    if (activeSort === key) setDescending((d) => !d);
    else {
      setSortKey(key);
      setDescending(true);
    }
  }
  const dirFor = (key: SortKey): SortDirection | undefined =>
    activeSort === key ? (descending ? "desc" : "asc") : undefined;

  // Counted, not hardcoded: a short span leaves the expanded row's background
  // stopping before the last column.
  const columnCount = 1 + (filter ? 2 : 1 + axes.length) + 1;

  if (people.viewer_unresolved) {
    return (
      <Card className="flex flex-col items-center justify-center gap-1 p-8 text-center">
        <p className="text-body text-foreground">We can&apos;t tell which prompts are yours</p>
        <p className="max-w-md text-body-sm text-muted-foreground">
          The email your coding tool reports isn&apos;t linked to your account yet, so none of
          these prompts can be attributed to you. Link it from your account settings and this
          fills in.
        </p>
      </Card>
    );
  }

  const header = (
    <div className="flex flex-wrap items-center gap-3">
      <h3 className="text-heading-4 text-foreground">
          {people.restricted_to_self ? "Your prompts" : "By person"}
        </h3>
        {filter && (
          <button
            type="button"
            onClick={onClearFilter}
            className="inline-flex items-center gap-1.5 rounded-full border border-border bg-surface px-2 py-0.5 text-mono-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: categoryColor(filter.axisKey, filter.colorIndex) }}
            />
            {filter.axisLabel}: {filter.labelName}
            <X className="size-3" aria-hidden />
            <span className="sr-only">Clear category filter</span>
          </button>
        )}
        {people.restricted_to_self && (
          <span className="inline-flex items-center gap-1.5 text-mono-sm text-faint-foreground">
            <Info className="size-3" aria-hidden />
            Account admins see everyone
          </span>
        )}
        {day && (
          <button
            type="button"
            onClick={onClearDay}
            className="inline-flex items-center gap-1.5 rounded-full border border-border bg-surface px-2 py-0.5 text-mono-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          >
            {formatDateShort(day)}
            <X className="size-3" aria-hidden />
            <span className="sr-only">Clear day filter</span>
          </button>
        )}
        {loading && <span className="text-mono-sm text-faint-foreground">Loading…</span>}
        {people.total_count > people.rows.length && (
          <span className="ml-auto text-mono-sm text-faint-foreground">
            Top {people.rows.length} of {people.total_count}
          </span>
        )}
    </div>
  );

  return (
    <Table header={header}>
        <TableHeader>
          <TableRow>
            <TableHead>Person</TableHead>
            {filter ? (
              <>
                <TableHead sortable sortDirection={dirFor("count")} onSort={() => sortBy("count")} className="text-right">
                  {filter.labelName} prompts
                </TableHead>
                <TableHead sortable sortDirection={dirFor("share")} onSort={() => sortBy("share")} className="text-right">
                  Share of their prompts
                </TableHead>
              </>
            ) : (
              <>
                <TableHead sortable sortDirection={dirFor("total")} onSort={() => sortBy("total")} className="text-right">
                  Prompts
                </TableHead>
                {axes.map((a) => {
                  const measure = measureFor(a.key);
                  const sortKeyForAxis: SortKey = `axis:${a.key}`;
                  const named = measure !== TOP_CATEGORY;
                  return (
                    <TableHead
                      key={a.key}
                      className="w-[220px]"
                      // Only a named label is a measure; a category name is not.
                      sortable={named}
                      sortDirection={named ? dirFor(sortKeyForAxis) : undefined}
                      onSort={named ? () => sortBy(sortKeyForAxis) : undefined}
                    >
                      <AxisMeasurePicker
                        axis={a}
                        measure={measure}
                        options={optionsByAxis[a.key] ?? []}
                        onChange={(next) => {
                          setMeasures((m) => ({ ...m, [a.key]: next }));
                          if (next !== TOP_CATEGORY) {
                            setSortKey(`axis:${a.key}`);
                            setDescending(true);
                          }
                        }}
                      />
                    </TableHead>
                  );
                })}
              </>
            )}
            <TableHead sortable sortDirection={dirFor("cost")} onSort={() => sortBy("cost")} className="text-right">
              Spend
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="py-8 text-center text-body-sm text-muted-foreground">
                {day
                  ? "Nobody matched on that day."
                  : "Nobody in this range used that category."}
              </TableCell>
            </TableRow>
          ) : (
            rows.map(({ row, count, share, cost }) => {
              const open = openKey === row.key;
              return (
              <Fragment key={row.key}>
              <TableRow
                onClick={() => setOpenKey(open ? null : row.key)}
                className="cursor-pointer"
                aria-expanded={open}
              >
                <TableCell className="max-w-[320px]">
                  <div className="flex min-w-0 items-center gap-1.5">
                    <ChevronRight
                      className={cn(
                        "size-3.5 shrink-0 text-muted-foreground transition-transform",
                        open && "rotate-90",
                      )}
                      aria-hidden
                    />
                    <IdentityLabel identity={row.identity} />
                  </div>
                </TableCell>
                {filter ? (
                  <>
                    <TableCell className="text-right font-mono text-body-sm tabular-nums">
                      {formatCompact(count)}
                    </TableCell>
                    <TableCell className="text-right">
                      <ShareBar pct={share} color={categoryColor(filter.axisKey, filter.colorIndex)} />
                    </TableCell>
                  </>
                ) : (
                  <>
                    <TableCell className="text-right font-mono text-body-sm tabular-nums">
                      {formatCompact(row.traces)}
                    </TableCell>
                    {axes.map((a) => (
                      <TableCell key={a.key}>
                        <AxisSummary
                          axisKey={a.key}
                          labels={row.axes[a.key]}
                          measure={measureFor(a.key)}
                        />
                      </TableCell>
                    ))}
                  </>
                )}
                <TableCell className="text-right font-mono text-body-sm tabular-nums text-muted-foreground">
                  {formatCost(cost)}
                </TableCell>
              </TableRow>
              {open && (
                <TableRow className="hover:bg-transparent">
                  <TableCell colSpan={columnCount} className="bg-surface p-0">
                    {/* Flex, not a 1fr grid: the rows are content-sized now, so
                        equal columns would park the second axis at the halfway
                        mark with a gap in front of it. */}
                    <div className="flex flex-wrap gap-x-10 gap-y-5 px-4 py-4">
                      {axes.map((a) => (
                        <PersonAxisBreakdown
                          key={a.key}
                          axisKey={a.key}
                          axisLabel={a.label}
                          labels={row.axes[a.key]}
                        />
                      ))}
                    </div>
                  </TableCell>
                </TableRow>
              )}
              </Fragment>
            );})
          )}
        </TableBody>
    </Table>
  );
}

function ShareBar({ pct, color }: { pct: number; color: string }) {
  return (
    <div className="flex items-center justify-end gap-2">
      <div className="h-1 w-20 overflow-hidden rounded-full bg-muted">
        <div
          className="h-full rounded-full"
          style={{ width: `${Math.min(Math.max(pct, 0), 100)}%`, backgroundColor: color }}
        />
      </div>
      <span className="w-9 text-right font-mono text-mono-sm tabular-nums text-foreground">
        {pct.toFixed(0)}%
      </span>
    </div>
  );
}

/** Top category by default; a named measure reports that label's share. */
function AxisSummary({
  axisKey,
  labels,
  measure,
}: {
  axisKey: string;
  labels: InsightsSourceLabel[] | undefined;
  measure: string;
}) {
  const segments = (labels ?? []).filter((l) => l.traces_pct > 0);
  if (segments.length === 0) return <span className="text-mono-sm text-faint-foreground">—</span>;

  // Zero is an answer — dropping the row would hide who is clean.
  const shown = measure === TOP_CATEGORY
    ? segments.reduce((a, b) => (b.traces_pct > a.traces_pct ? b : a))
    : labelFor(labels, measure);
  const pct = shown?.traces_pct ?? 0;
  const color = categoryColor(axisKey, shown?.color_index ?? 0);

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-baseline gap-1.5">
        <span
          className="size-2 shrink-0 translate-y-px rounded-full"
          style={{ backgroundColor: color }}
        />
        <span className="min-w-0 truncate text-body-sm text-foreground">
          {shown?.label ?? "None"}
        </span>
        <span className="ml-auto shrink-0 font-mono text-mono-sm tabular-nums text-muted-foreground">
          {pct.toFixed(0)}%
        </span>
      </div>
      <div className="flex h-1 w-full overflow-hidden rounded-full bg-muted">
        {measure === TOP_CATEGORY
          ? segments.map((l) => (
            <div
              key={l.key}
              className="h-full first:rounded-l-full last:rounded-r-full"
              style={{ width: `${l.traces_pct}%`, backgroundColor: categoryColor(axisKey, l.color_index) }}
            />
          ))
          : (
            <div
              className="h-full rounded-full"
              style={{ width: `${Math.min(pct, 100)}%`, backgroundColor: color }}
            />
          )}
      </div>
    </div>
  );
}

/** Every label present on an axis, in declared order, for the column selector. */
function axisLabelOptions(
  rows: { axes: Record<string, InsightsSourceLabel[]> }[],
  axisKey: string,
): InsightsSourceLabel[] {
  const seen = new Map<string, InsightsSourceLabel>();
  for (const row of rows) {
    for (const l of row.axes[axisKey] ?? []) {
      if (!seen.has(l.key)) seen.set(l.key, l);
    }
  }
  return [...seen.values()].sort((a, b) => a.color_index - b.color_index);
}

function AxisMeasurePicker({
  axis,
  measure,
  options,
  onChange,
}: {
  axis: { key: string; label: string };
  measure: string;
  options: InsightsSourceLabel[];
  onChange: (next: string) => void;
}) {
  const active = measure === TOP_CATEGORY ? null : options.find((o) => o.key === measure);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          // Keep the click off the header's sort handler.
          onClick={(e) => e.stopPropagation()}
          className="inline-flex max-w-full items-center gap-1 rounded text-body-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:bg-muted focus-visible:outline-none"
        >
          <span className="truncate">{active ? `% ${active.label}` : axis.label}</span>
          <ChevronDown className="size-3 shrink-0" aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-72 overflow-y-auto">
        <DropdownMenuRadioGroup value={measure} onValueChange={onChange}>
          <DropdownMenuRadioItem value={TOP_CATEGORY}>Top category</DropdownMenuRadioItem>
          {options.map((o) => (
            <DropdownMenuRadioItem key={o.key} value={o.key}>
              % {o.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** A person's full distribution on one axis, sorted by share. Read-only:
 *  filtering lives on the charts, where the click and the selection match. */
function PersonAxisBreakdown({
  axisKey,
  axisLabel,
  labels,
}: {
  axisKey: string;
  axisLabel: string;
  labels: InsightsSourceLabel[] | undefined;
}) {
  const segments = useMemo(
    () => [...(labels ?? [])].sort((a, b) => b.traces_pct - a.traces_pct),
    [labels],
  );
  if (segments.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        <h4 className="text-mono-sm uppercase tracking-wide text-faint-foreground">{axisLabel}</h4>
        <p className="text-body-sm text-muted-foreground">No prompts categorised on this axis.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <h4 className="text-mono-sm uppercase tracking-wide text-faint-foreground">{axisLabel}</h4>
      <ul className="flex flex-col gap-1">
        {segments.map((l) => (
          <li key={l.key} className="flex items-center gap-2 px-1.5">
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: categoryColor(axisKey, l.color_index) }}
            />
            {/* Fixed width, not flex-1: letting the label grow pushed the
                numbers to the far edge and left a gulf across the row. Fixed
                keeps the columns tight together and still aligned row to row. */}
            <span className="w-36 shrink-0 truncate text-body-sm text-foreground">{l.label}</span>
            <span className="w-10 shrink-0 text-right font-mono text-mono-sm tabular-nums text-muted-foreground">
              {formatCompact(l.traces)}
            </span>
            <div className="hidden h-1 w-14 shrink-0 overflow-hidden rounded-full bg-muted @lg:block">
              <div
                className="h-full rounded-full"
                style={{
                  width: `${Math.min(l.traces_pct, 100)}%`,
                  backgroundColor: categoryColor(axisKey, l.color_index),
                }}
              />
            </div>
            <span className="w-8 shrink-0 text-right font-mono text-mono-sm tabular-nums text-foreground">
              {l.traces_pct.toFixed(0)}%
            </span>
            <span className="w-12 shrink-0 text-right font-mono text-mono-sm tabular-nums text-faint-foreground">
              {formatCost(l.cost_usd)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
