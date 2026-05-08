import type { Blueprint } from "@/lib/api";
import { BlueprintCard } from "@/components/BlueprintCard";
import { getBlueprintDescription } from "@/lib/blueprint-utils";
import { TabSearchInput, TabFilterDropdown } from "./TabToolbar";

export type VisibilityFilter = "all" | "public" | "private";
export type BlueprintSort = "newest" | "name" | "deployed";

const VISIBILITY_OPTIONS: { value: VisibilityFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "public", label: "Public" },
  { value: "private", label: "Private" },
];

const SORT_OPTIONS: { value: BlueprintSort; label: string }[] = [
  { value: "newest", label: "Newest" },
  { value: "name", label: "Name A–Z" },
  { value: "deployed", label: "Most deployed" },
];

interface BlueprintsTabProps {
  blueprints: Blueprint[];
  accountName: string;
  isOwner: boolean;
  isInternalView: boolean;
  search: string;
  onSearchChange: (v: string) => void;
  visibility: VisibilityFilter;
  onVisibilityChange: (v: VisibilityFilter) => void;
  sort: BlueprintSort;
  onSortChange: (v: BlueprintSort) => void;
}

export function BlueprintsTab({
  blueprints,
  accountName,
  isOwner,
  isInternalView,
  search,
  onSearchChange,
  visibility,
  onVisibilityChange,
  sort,
  onSortChange,
}: BlueprintsTabProps) {
  const hasFilters = search.trim() !== "" || visibility !== "all" || sort !== "newest";
  const visibilityLabel = visibility === "all" ? "Visibility" : visibility === "public" ? "Public" : "Private";
  const sortLabel = SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Newest";

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-3 flex-wrap">
        <TabSearchInput value={search} onChange={onSearchChange} placeholder="Search blueprints…" />

        {isInternalView && (
          <TabFilterDropdown
            value={visibility}
            onChange={onVisibilityChange}
            options={VISIBILITY_OPTIONS}
            triggerLabel={visibilityLabel}
            minWidth="min-w-32"
          />
        )}

        <TabFilterDropdown
          value={sort}
          onChange={onSortChange}
          options={SORT_OPTIONS}
          triggerLabel={sortLabel}
        />
      </div>

      {blueprints.length === 0 ? (
        <p className="text-body text-muted-foreground">
          {hasFilters ? "No blueprints match your filters." : "No blueprints published yet."}
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {blueprints.map((agent) => (
            <BlueprintCard
              key={agent.name}
              slug={`${accountName}/${agent.name}`}
              account={accountName}
              name={agent.name}
              description={getBlueprintDescription(agent)}
              visibility={agent.visibility}
              avatarColors={agent.avatar_colors}
              deployCount={agent.metrics?.deploy_count}
              onArchive={isOwner ? () => {} : undefined}
            />
          ))}
        </div>
      )}
    </div>
  );
}
