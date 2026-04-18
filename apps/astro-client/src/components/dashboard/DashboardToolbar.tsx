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


const STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: "active", label: deploymentStatusLabel.active },
  { value: "deploying", label: deploymentStatusLabel.deploying },
  { value: "error", label: deploymentStatusLabel.error },
  { value: "inactive", label: deploymentStatusLabel.inactive },
];

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
    <div
      className={cn(
        "flex flex-wrap items-center gap-2",
        disabled && "pointer-events-none opacity-40",
      )}
    >
      <FilterInput
        placeholder="Search agents..."
        value={filter}
        onChange={(e) => onFilterChange(e.target.value)}
        containerClassName="w-full @[480px]:flex-1 @[480px]:max-w-lg h-8 !bg-white dark:!bg-background"
      />

      <MultiSelect value={statusFilter} onValueChange={onStatusFilterChange}>
        <MultiSelectTrigger className="h-8 w-full @[480px]:w-36 text-sm">
          <MultiSelectValue
            options={STATUS_OPTIONS}
            placeholder="All statuses"
          />
        </MultiSelectTrigger>
        <MultiSelectContent>
          <MultiSelectAllItem>All statuses</MultiSelectAllItem>
          {STATUS_OPTIONS.map((opt) => (
            <MultiSelectItem key={opt.value} value={opt.value}>
              {opt.label}
            </MultiSelectItem>
          ))}
        </MultiSelectContent>
      </MultiSelect>

      <Select
        value={sortBy}
        onValueChange={(v) => onSortChange(v as SortOption)}
      >
        <SelectTrigger className="h-8 w-full @[480px]:w-36 px-3 text-sm !bg-white dark:!bg-background">
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
