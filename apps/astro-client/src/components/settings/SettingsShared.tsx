import { RefreshCw } from "lucide-react";
import React from "react";
import { CheckIcon } from "@heroicons/react/24/outline";
import { Skeleton } from "@/components/ui/skeleton";
import { ActionPanel } from "@/components/ui/status-panel";
import { cn } from "@/lib/utils";

const headingClass = {
  h1: "text-heading-1",
  h2: "text-heading-2",
  h3: "text-heading-3",
} as const;

export function SectionHeader({
  as: Heading = "h2",
  title,
  subtitle,
  action,
  className,
}: {
  as?: "h1" | "h2" | "h3";
  title: React.ReactNode;
  subtitle: string;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("pb-4 mb-4 border-b border-border", className)}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div className="space-y-1 min-w-0">
          <Heading className={`${headingClass[Heading]} text-foreground`}>{title}</Heading>
          <p className="text-[13px] text-pretty text-muted-foreground">{subtitle}</p>
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
    </div>
  );
}

export function SavedIndicator({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <span className="flex items-center gap-1 text-[13px] text-muted-foreground">
      <CheckIcon className="size-3.5" />
      Saved
    </span>
  );
}

/** Placeholder rows shaped like the content they stand in for, so a section
 *  keeps its height and the page does not jump when the data lands. */
export function LoadingRows({ rows = 3, className }: { rows?: number; className?: string }) {
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-9 w-full" />
      ))}
    </div>
  );
}

export function EmptyState({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-border bg-surface px-5 py-4">
      <p className="text-body-sm text-muted-foreground">{message}</p>
    </div>
  );
}

export function Unavailable() {
  return (
    <EmptyState message="Billing isn't available for this account yet. Data appears here once billing is enabled." />
  );
}

// A query failed (network error, 5xx), distinct from data.available being
// false; conflating the two would tell a user mid-outage that billing is off.
export function LoadError({
  message = "Couldn't load this.",
  onRetry,
}: {
  message?: string;
  onRetry: () => void;
}) {
  return <ActionPanel tone="error" title={message} primaryLabel="Retry" onPrimary={onRetry} />;
}

export function RefreshButton({ onRefresh, busy }: { onRefresh: () => void; busy: boolean }) {
  return (
    <button
      type="button"
      onClick={onRefresh}
      disabled={busy}
      aria-label="Refresh billing"
      className="inline-flex items-center gap-1.5 rounded-sm border border-border px-3 py-1.5 text-body-sm text-foreground hover:bg-muted disabled:cursor-default disabled:hover:bg-transparent"
    >
      <RefreshCw className={cn("size-3.5 text-muted-foreground", busy && "animate-spin")} aria-hidden />
      Refresh
    </button>
  );
}
