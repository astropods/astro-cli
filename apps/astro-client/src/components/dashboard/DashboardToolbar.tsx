import { FilterInput } from "@/components/FilterInput";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import {
  MultiSelect,
  MultiSelectTrigger,
  MultiSelectValue,
  MultiSelectContent,
  MultiSelectAllItem,
  MultiSelectItem,
} from "@/components/ui/multi-select";
import { deploymentStatusLabel } from "@/lib/deployment-utils";
import type { SortOption } from "./useAgentFilters";

export type { SortOption };

const STATUS_COLORS: Record<string, string> = {
  active: "var(--color-teal-600)",
  inactive: "var(--color-stone-400)",
  pending: "var(--color-yellow-500)",
  error: "var(--color-coral-600)",
  undeploying: "var(--color-stone-400)",
};

const STATUS_OPTIONS = Object.entries(deploymentStatusLabel).map(([value, label]) => ({
  value,
  label,
  color: STATUS_COLORS[value],
}));

const SORT_OPTIONS: { label: string; value: SortOption }[] = [
  { label: "Last updated", value: "recent" },
  { label: "Name (A-Z)", value: "name" },
  { label: "Most requests", value: "requests" },
];

interface DashboardToolbarProps {
  filter: string;
  onFilterChange: (v: string) => void;
  statusFilter: string[];
  onStatusFilterChange: (v: string[]) => void;
  sortBy: SortOption;
  onSortChange: (v: SortOption) => void;
  disabled?: boolean;
}

export function DashboardToolbar({
  filter,
  onFilterChange,
  statusFilter,
  onStatusFilterChange,
  sortBy,
  onSortChange,
  disabled,
}: DashboardToolbarProps) {
  return (
    <div className={cn("flex items-center gap-2", disabled && "pointer-events-none opacity-40")}>
      <FilterInput
        placeholder="Search agents..."
        value={filter}
        onChange={(e) => onFilterChange(e.target.value)}
        containerClassName="flex-1 max-w-lg h-8 !bg-white dark:!bg-background"
      />

      <MultiSelect value={statusFilter} onValueChange={onStatusFilterChange}>
        <MultiSelectTrigger className="h-8 w-36 text-xs">
          <MultiSelectValue options={STATUS_OPTIONS} placeholder="All statuses" />
        </MultiSelectTrigger>
        <MultiSelectContent>
          <MultiSelectAllItem>All statuses</MultiSelectAllItem>
          {STATUS_OPTIONS.map((opt) => (
            <MultiSelectItem key={opt.value} value={opt.value} color={opt.color}>
              {opt.label}
            </MultiSelectItem>
          ))}
        </MultiSelectContent>
      </MultiSelect>

      <Select value={sortBy} onValueChange={(v) => onSortChange(v as SortOption)}>
        <SelectTrigger className="h-8 w-36 px-3 text-xs bg-background">
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
  );
}
