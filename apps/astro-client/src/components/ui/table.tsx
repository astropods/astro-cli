import * as React from "react"
import { ChevronDown } from "lucide-react"

import { cn } from "@/lib/utils"

function Table({
  className,
  containerClassName,
  children,
  header,
  footer,
  bare,
  ...props
}: React.ComponentProps<"table"> & {
  containerClassName?: string;
  /** Optional content rendered inside the bordered container, above the
   *  table — used for a panel title + toolbar (e.g. view toggle + search).
   *  Separated from the table by a thin border so the chrome reads as one
   *  unified card without nesting a second border. */
  header?: React.ReactNode;
  /** Optional content rendered inside the bordered container, below the
   *  table — used for pagination affordances like <TableShowMore />. */
  footer?: React.ReactNode;
  /** Drop the rounded-card chrome (outer border + rounded corners +
   *  overflow clip) so the table flows in the page surface. The header
   *  row + per-row border-b still render. */
  bare?: boolean;
}) {
  return (
    <div
      data-slot="table-container"
      className={cn(!bare && "rounded-sm border border-border overflow-hidden", containerClassName)}
    >
      {header && (
        <div
          data-slot="table-header-bar"
          className={cn("px-4 py-3", !bare && "border-b border-border")}
        >
          {header}
        </div>
      )}
      <div className="relative w-full overflow-auto">
        <table
          data-slot="table"
          className={cn("w-full border-collapse text-body leading-5", className)}
          {...props}
        >
          {children}
        </table>
      </div>
      {footer && (
        <div
          data-slot="table-footer-bar"
          className={cn(!bare && "border-t border-border")}
        >
          {footer}
        </div>
      )}
    </div>
  )
}

function TableShowMore({
  hiddenCount,
  expanded,
  onToggle,
  className,
  showMoreLabel,
  revealedCount = 0,
  onShowLess,
  showLessLabel = "Show less",
}: {
  hiddenCount: number;
  expanded: boolean;
  onToggle: () => void;
  className?: string;
  /** Optional label for the reveal action, e.g. "Show top 10" or "Show all". */
  showMoreLabel?: string;
  /** Rows already revealed past the default window; enables a collapse affordance. */
  revealedCount?: number;
  onShowLess?: () => void;
  /** Optional label for collapsing back to the default row window. */
  showLessLabel?: string;
}) {
  if (hiddenCount <= 0 && revealedCount <= 0) return null;

  if (showMoreLabel || revealedCount > 0) {
    return (
      <div
        className={cn(
          "flex w-full items-center justify-center gap-2 py-2.5 text-mono-sm",
          className,
        )}
      >
        {hiddenCount > 0 && (
          <button
            type="button"
            onClick={onToggle}
            className="inline-flex items-center justify-center gap-1.5 rounded px-2 py-1 text-muted-foreground transition-colors hover:text-foreground"
          >
            <ChevronDown aria-hidden className="size-3.5" />
            {showMoreLabel ?? `Show ${hiddenCount} more`}
          </button>
        )}
        {revealedCount > 0 && (
          <button
            type="button"
            onClick={onShowLess ?? onToggle}
            className="inline-flex items-center justify-center gap-1.5 rounded px-2 py-1 text-faint-foreground transition-colors hover:text-foreground"
          >
            <ChevronDown aria-hidden className="size-3.5 rotate-180" />
            {showLessLabel}
          </button>
        )}
      </div>
    );
  }

  if (hiddenCount <= 0) return null;
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        "flex w-full items-center justify-center gap-1.5 py-3 text-mono-sm text-muted-foreground transition-colors hover:text-foreground",
        className,
      )}
    >
      <ChevronDown
        aria-hidden
        className={cn("size-3.5 transition-transform", expanded && "rotate-180")}
      />
      {expanded ? showLessLabel : `Show ${hiddenCount} more`}
    </button>
  );
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("border-b border-border bg-muted/40 dark:bg-foreground/5", className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn(className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t border-border font-medium [&>tr]:border-0",
        className,
      )}
      {...props}
    />
  )
}

function TableRow({
  className,
  interactive,
  ...props
}: React.ComponentProps<"tr"> & { interactive?: boolean }) {
  return (
    <tr
      data-slot="table-row"
      data-interactive={interactive ? "true" : undefined}
      className={cn(
        "border-b border-border last:border-b-0",
        interactive && "cursor-pointer transition-colors hover:bg-muted/20 dark:hover:bg-accent/30",
        className,
      )}
      {...props}
    />
  )
}

export type SortDirection = "asc" | "desc"

function TableHead({
  className,
  sortable,
  sortDirection,
  onSort,
  onClick,
  children,
  ...props
}: React.ComponentProps<"th"> & {
  /** Render a sort indicator and make the header clickable. */
  sortable?: boolean
  /** Current sort direction for this column. Omit when sortable but not the
   *  current sort target — no indicator is rendered (only the active column
   *  carries an arrow). */
  sortDirection?: SortDirection
  /** Fired on click when sortable. Caller decides which direction to toggle to. */
  onSort?: () => void
}) {
  return (
    <th
      data-slot="table-head"
      className={cn(
        "px-4 py-2 align-bottom text-left text-body-sm font-medium text-muted-foreground md:align-middle",
        sortable && "cursor-pointer select-none transition-colors hover:text-foreground",
        className,
      )}
      onClick={sortable && onSort ? (e) => { onClick?.(e); onSort() } : onClick}
      aria-sort={
        sortable
          ? sortDirection === "asc"
            ? "ascending"
            : sortDirection === "desc"
              ? "descending"
              : "none"
          : undefined
      }
      {...props}
    >
      {children}
      {sortable && sortDirection && (
        <span aria-hidden className="ml-1 inline-block text-foreground">
          {sortDirection === "asc" ? "↑" : "↓"}
        </span>
      )}
    </th>
  )
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      data-slot="table-cell"
      className={cn("px-4 py-2 align-middle", className)}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("text-muted-foreground mt-4 text-body-sm", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableRow,
  TableHead,
  TableCell,
  TableCaption,
  TableShowMore,
}
