import { AccountFilter } from "@/components/AccountFilter";
import { FilterInput } from "@/components/FilterInput";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { SortOption } from "./useAgentFilters";

export type { SortOption };

const SORT_OPTIONS: { label: string; value: SortOption }[] = [
  { label: "Last updated", value: "recent" },
  { label: "Name (A-Z)", value: "name" },
  { label: "Most requests", value: "requests" },
];

interface DashboardToolbarProps {
  filter: string;
  onFilterChange: (v: string) => void;
  sortBy: SortOption;
  onSortChange: (v: SortOption) => void;
  disabled?: boolean;
  accountFilters: string[];
  onAccountFiltersChange: (accounts: string[]) => void;
}

export function DashboardToolbar({
  filter,
  onFilterChange,
  sortBy,
  onSortChange,
  disabled,
  accountFilters,
  onAccountFiltersChange,
}: DashboardToolbarProps) {
  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-2",
        disabled && "pointer-events-none opacity-40",
      )}
    >
      <FilterInput
        placeholder="Search agents..."
        value={filter}
        onChange={(e) => onFilterChange(e.target.value)}
        containerClassName="w-full @[480px]:w-auto @[480px]:flex-1 @[480px]:max-w-lg h-8 bg-card dark:bg-background"
      />

      <div className="flex w-full flex-col gap-2 @[480px]:w-auto @[480px]:flex-row @[480px]:items-center">
        <Select
          value={sortBy}
          onValueChange={(v) => onSortChange(v as SortOption)}
        >
          <SelectTrigger className="h-8 w-full @[480px]:w-36 px-3 text-sm bg-card dark:bg-background">
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
        <AccountFilter
          className="w-full @[480px]:w-auto @[480px]:min-w-[13rem]"
          value={accountFilters}
          onChange={onAccountFiltersChange}
        />
      </div>
    </div>
  );
}
