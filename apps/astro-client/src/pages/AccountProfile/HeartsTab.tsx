import { useState, useCallback, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { BlueprintCard } from "@/components/BlueprintCard";
import { useHeartedBlueprints, useHeartToggleInList } from "@/api/queries/hearts";
import { TabSearchInput, TabFilterDropdown } from "./TabToolbar";

export type HeartSort = "popular" | "name";

const SORT_OPTIONS: { value: HeartSort; label: string }[] = [
  { value: "popular", label: "Most hearts" },
  { value: "name", label: "Name A–Z" },
];

interface HeartsTabProps {
  accountName: string;
  isOwner: boolean;
  search: string;
  onSearchChange: (v: string) => void;
  sort: HeartSort;
  onSortChange: (v: HeartSort) => void;
}

export function HeartsTab({ accountName, isOwner, search, onSearchChange, sort, onSortChange }: HeartsTabProps) {
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [allCursors, setAllCursors] = useState<(string | undefined)[]>([undefined]);
  const [localHearts, setLocalHearts] = useState<Map<string, boolean>>(new Map());

  const { data, isLoading } = useHeartedBlueprints(accountName, cursor);
  const heartToggle = useHeartToggleInList();

  // Reset to first page whenever the user changes the filter or sort
  useEffect(() => {
    setCursor(undefined);
    setAllCursors([undefined]);
  }, [search, sort]);

  const isHearted = useCallback(
    (account: string, name: string) => localHearts.get(`${account}/${name}`) ?? true,
    [localHearts],
  );

  const handleToggle = useCallback((account: string, name: string) => {
    const key = `${account}/${name}`;
    setLocalHearts((prev) => {
      const current = prev.get(key) ?? true;
      return new Map(prev).set(key, !current);
    });
    heartToggle.mutate({ account, name }, {
      onError: () => {
        setLocalHearts((prev) => {
          const current = prev.get(key) ?? false;
          return new Map(prev).set(key, !current);
        });
      },
    });
  }, [heartToggle]);

  const sortLabel = SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Most hearts";
  const hasFilters = search.trim() !== "" || sort !== "popular";

  const items = data?.items ?? [];
  const q = search.trim().toLowerCase();

  const filtered = items
    .filter((item) => !q || item.name.toLowerCase().includes(q))
    .sort((a, b) => {
      if (sort === "name") return a.name.localeCompare(b.name);
      if (sort === "popular") return b.heart_count - a.heart_count;
      return 0;
    });

  function goNext() {
    if (!data?.next_cursor) return;
    const next = data.next_cursor;
    setAllCursors((prev) => [...prev, next]);
    setCursor(next);
  }

  function goPrev() {
    if (allCursors.length <= 1) return;
    const prev = allCursors[allCursors.length - 2];
    setAllCursors((c) => c.slice(0, -1));
    setCursor(prev);
  }

  const hasPrev = allCursors.length > 1;
  const hasNext = !!data?.next_cursor;

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-3">
        <TabSearchInput value={search} onChange={onSearchChange} placeholder="Filter this page…" />
        <TabFilterDropdown
          value={sort}
          onChange={onSortChange}
          options={SORT_OPTIONS}
          triggerLabel={sortLabel}
        />
      </div>

      {isLoading && filtered.length === 0 ? null : filtered.length === 0 ? (
        <p className="text-body text-muted-foreground">
          {hasFilters ? "No hearts match your search." : "No hearted blueprints yet."}
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {filtered.map((item) => (
            <div key={`${item.account}/${item.name}`} className="relative h-full">
              <BlueprintCard
                slug={`${item.account}/${item.name}`}
                account={item.account}
                name={item.name}
                description={item.description || item.name}
                visibility={item.visibility}
                avatarColors={item.avatar_colors}
                deployCount={item.deploy_count}
                heartCount={item.heart_count}
                onHeartToggle={isOwner ? () => handleToggle(item.account, item.name) : undefined}
                isHearted={isOwner ? isHearted(item.account, item.name) : undefined}
              />
            </div>
          ))}
        </div>
      )}

      {(hasPrev || hasNext) && (
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={goPrev} disabled={!hasPrev}>
            Previous
          </Button>
          <Button variant="outline" size="sm" onClick={goNext} disabled={!hasNext}>
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
