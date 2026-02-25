import { Link } from "react-router";
import { Grid2x2, List, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { FilterInput } from "@/components/FilterInput";

export type ViewMode = "list" | "grid";

export interface MyAgentsHeaderProps {
  filter: string;
  onFilterChange: (value: string) => void;
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
}

export function MyAgentsHeader({
  filter,
  onFilterChange,
  viewMode,
  onViewModeChange,
}: MyAgentsHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-4">
        <h2 className="text-[32px] font-semibold leading-tight">My agents</h2>
        <FilterInput
          placeholder="Filter agents…"
          className="w-[248px]"
          value={filter}
          onChange={(e) => onFilterChange(e.target.value)}
        />
      </div>

      <div className="flex items-center gap-3">
        <ToggleGroup
          type="single"
          value={viewMode}
          onValueChange={(value) => {
            if (value) onViewModeChange(value as ViewMode);
          }}
        >
          <ToggleGroupItem value="grid" aria-label="Card view" tooltip="Card view">
            <Grid2x2 className="h-4 w-4" />
          </ToggleGroupItem>
          <ToggleGroupItem value="list" aria-label="List view" tooltip="List view">
            <List className="h-4 w-4" />
          </ToggleGroupItem>
        </ToggleGroup>

        <Button asChild>
          <Link to="/hire">
            <Plus className="h-4 w-4" />
            Add agents
          </Link>
        </Button>
      </div>
    </div>
  );
}
