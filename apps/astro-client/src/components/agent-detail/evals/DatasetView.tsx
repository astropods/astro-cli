import { useCallback, useState, type RefObject } from "react";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";
import { DatasetTable } from "./DatasetTable";
import { DatasetFilterChips, type FilterKey } from "./DatasetFilterChips";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "./EvalTabCard";
import type { EvalDatasetResponse } from "@/lib/api";

export interface DatasetViewProps {
  deploymentId: string;
  account: string;
  summary: EvalDatasetResponse;
  reviewQueueTargetRef?: RefObject<HTMLElement | null>;
}

export function DatasetView({
  deploymentId,
  account,
  summary,
  reviewQueueTargetRef,
}: DatasetViewProps) {
  const [selected, setSelected] = useState<Set<FilterKey>>(() => new Set());

  const toggleFilter = useCallback((key: FilterKey) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }, []);

  return (
    <EvalTabCard className="@container/dataset-card">
      <EvalTabCardHeader label="Dataset" datasetName={summary.dataset_name}>
        <DatasetFilterChips
          selected={selected}
          counts={{ good: summary.good_count, bad: summary.bad_count }}
          onToggle={toggleFilter}
        />
      </EvalTabCardHeader>
      <EvalTabCardBody className="flex-col @[780px]/dataset-card:flex-row">
        <DatasetGradeSidebar summary={summary} />
        <DatasetTable
          deploymentId={deploymentId}
          account={account}
          summary={summary}
          selected={selected}
          reviewQueueTargetRef={reviewQueueTargetRef}
        />
      </EvalTabCardBody>
    </EvalTabCard>
  );
}
