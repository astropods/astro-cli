import { useCallback, useState } from "react";
import { PillToggle } from "@/components/activity/PillToggle";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";
import { DatasetTable } from "./DatasetTable";
import { DatasetFilterChips, type FilterKey } from "./DatasetFilterChips";
import type { RawMode } from "./DatasetItemRow";
import type { EvalDatasetResponse } from "@/lib/api";

export interface DatasetViewProps {
  deploymentId: string;
  account: string;
  summary: EvalDatasetResponse;
}

export function DatasetView({
  deploymentId,
  account,
  summary,
}: DatasetViewProps) {
  const [selected, setSelected] = useState<Set<FilterKey>>(() => new Set());
  const [rawMode, setRawMode] = useState<RawMode>("pretty");

  const toggleFilter = useCallback((key: FilterKey) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  return (
    <div className="flex flex-col overflow-hidden rounded-lg border border-border">
      <div className="flex items-center justify-between gap-4 border-b border-border bg-card px-5 py-3 dark:bg-surface">
        <span className="truncate text-body font-semibold text-foreground">
          {summary.dataset_name}
        </span>
        <div className="flex items-center gap-3">
          <DatasetFilterChips
            selected={selected}
            counts={{ good: summary.good_count, bad: summary.bad_count }}
            onToggle={toggleFilter}
          />
          <span aria-hidden className="h-[22px] w-px bg-border" />
          <PillToggle<RawMode>
            layoutId="dataset-raw-mode"
            value={rawMode}
            onChange={setRawMode}
            size="md"
            options={[
              { key: "pretty", label: "Pretty" },
              { key: "raw", label: "Raw" },
            ]}
          />
        </div>
      </div>

      <div className="flex">
        <DatasetGradeSidebar summary={summary} />
        <DatasetTable
          deploymentId={deploymentId}
          account={account}
          summary={summary}
          selected={selected}
          rawMode={rawMode}
        />
      </div>
    </div>
  );
}
