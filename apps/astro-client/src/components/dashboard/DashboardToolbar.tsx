import { DebouncedFilterInput } from "@/components/DebouncedFilterInput";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { SortOption, DeploymentStatusFilter } from "./useAgentFilters";

export type { SortOption };

const SORT_OPTIONS: { label: string; value: SortOption }[] = [
  { label: "Last updated", value: "recent" },
  { label: "Name (A-Z)", value: "name" },
  { label: "Most requests", value: "requests" },
];

const STATUS_FILTERS: { label: string; value: DeploymentStatusFilter | null }[] = [
  { label: "All", value: null },
  { label: "Active", value: "active" },
  { label: "Stopped", value: "stopped" },
  { label: "Error", value: "error" },
];

interface DashboardToolbarProps {
  filter: string;
  onFilterChange: (v: string) => void;
  sortBy: SortOption;
  onSortChange: (v: SortOption) => void;
  statusFilter?: DeploymentStatusFilter | null;
  onStatusChange?: (v: DeploymentStatusFilter | null) => void;
  disabled?: boolean;
  filterResetKey?: number;
}

export function DashboardToolbar({
  filter,
  onFilterChange,
  sortBy,
  onSortChange,
  statusFilter = null,
  onStatusChange,
  disabled,
  filterResetKey,
}: DashboardToolbarProps) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-2",
        disabled && "pointer-events-none opacity-40",
      )}
    >
      <DebouncedFilterInput
        placeholder="Search agents..."
        value={filter}
        resetKey={filterResetKey}
        onDebouncedChange={onFilterChange}
        containerClassName="w-full @[480px]:w-auto @[480px]:flex-1 @[480px]:max-w-lg h-8 bg-card dark:bg-background"
      />

      <div className="flex w-full flex-col gap-2 @[480px]:w-auto @[480px]:flex-row @[480px]:items-center">
        {onStatusChange && (
          <div
            role="group"
            aria-label="Filter by status"
            className="flex items-center rounded-md border border-border bg-card p-0.5 dark:bg-background"
          >
            {STATUS_FILTERS.map((s) => {
              const active = (statusFilter ?? null) === s.value;
              return (
                <button
                  key={s.label}
                  type="button"
                  aria-pressed={active}
                  onClick={() => onStatusChange(s.value)}
                  className={cn(
                    "rounded-[5px] px-2.5 py-1 text-sm transition-colors",
                    active
                      ? "bg-accent text-foreground"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {s.label}
                </button>
              );
            })}
          </div>
        )}
        <Select
          value={sortBy}
          onValueChange={(v) => onSortChange(v as SortOption)}
        >
          <SelectTrigger
            aria-label="Sort current page"
            title="Sort current page"
            className="h-8 w-full @[480px]:w-36 px-3 text-sm bg-card dark:bg-background"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SORT_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
