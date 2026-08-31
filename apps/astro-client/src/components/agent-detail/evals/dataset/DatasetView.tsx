import type { RefObject } from "react";
import { DatasetGradeSidebar } from "./DatasetGradeSidebar";
import { DatasetTable } from "./DatasetTable";
import { EvalTabCard, EvalTabCardBody, EvalTabCardHeader } from "../EvalTabCard";
import { useAgentEvaluationSet } from "@/api/queries/evals";
import type { EvalDatasetResponse } from "@/lib/api";

interface DatasetViewProps {
  deploymentId: string;
  account: string;
  agentName: string;
  summary: EvalDatasetResponse;
  reviewQueueTargetRef?: RefObject<HTMLElement | null>;
}

export function DatasetView({
  deploymentId,
  account,
  agentName,
  summary,
  reviewQueueTargetRef,
}: DatasetViewProps) {
  const {
    data: evaluationSet,
    isLoading: evaluatorsLoading,
    isError: evaluatorsError,
  } = useAgentEvaluationSet(account, agentName);
  const evaluators = evaluationSet?.evaluators ?? [];
  const evaluatorsUnavailable =
    evaluatorsLoading || evaluatorsError || evaluators.length === 0;

  return (
    <EvalTabCard className="@container/dataset-card">
      <EvalTabCardHeader label="Dataset" />
      <EvalTabCardBody className="flex-col @[780px]/dataset-card:flex-row">
        <DatasetGradeSidebar summary={summary} />
        <DatasetTable
          deploymentId={deploymentId}
          account={account}
          summary={summary}
          evaluators={evaluators}
          evaluationRef={evaluationSet?.evaluation_ref}
          evaluatorsUnavailable={evaluatorsUnavailable}
          reviewQueueTargetRef={reviewQueueTargetRef}
        />
      </EvalTabCardBody>
    </EvalTabCard>
  );
}
