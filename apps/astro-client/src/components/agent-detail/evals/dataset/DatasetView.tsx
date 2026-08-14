import type { RefObject } from "react";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";
import { DatasetTable } from "./DatasetTable";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "../EvalTabCard";
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
  return (
    <EvalTabCard className="@container/dataset-card">
      <EvalTabCardHeader label="Dataset" />
      <EvalTabCardBody className="flex-col @[780px]/dataset-card:flex-row">
        <DatasetGradeSidebar summary={summary} />
        <DatasetTable
          deploymentId={deploymentId}
          account={account}
          summary={summary}
          reviewQueueTargetRef={reviewQueueTargetRef}
        />
      </EvalTabCardBody>
    </EvalTabCard>
  );
}
