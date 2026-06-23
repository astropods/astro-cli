import { Download } from "lucide-react";
import { useAgentDetailContext } from "../AgentDetail";
import { useEvalDataset } from "@/api/queries/deployments";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { DatasetGrade } from "@/components/agent-detail/evals/DatasetGrade";
import { DatasetView } from "@/components/agent-detail/evals/DatasetView";

export default function AgentDataset() {
  const { deploymentId, account } = useAgentDetailContext();
  const { data, isLoading, isError } = useEvalDataset(deploymentId);

  const downloadUrl = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/download`;

  return (
    <div className="relative z-10 flex flex-1 flex-col pt-16">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 px-6 py-8 pb-16">
          <div className="flex items-end justify-between gap-6">
            <div className="min-w-0">
              <h1 className="text-heading-1 text-foreground">Eval</h1>
              <p className="mt-1.5 max-w-[66ch] text-body-sm text-muted-foreground">
                Measure how well your agent performs by testing it against a set
                of examples.
              </p>
            </div>
            {data && (
              <div className="flex flex-none items-center gap-2.5">
                <div className="flex items-center gap-2 pr-1.5">
                  <span className="text-body-sm text-muted-foreground">
                    Grade
                  </span>
                  <DatasetGrade grade={data.grade} />
                </div>
                <span aria-hidden className="h-5 w-px bg-border" />
                <Button asChild variant="outline" size="sm">
                  <a href={downloadUrl} download>
                    <Download className="size-4" />
                    Download
                  </a>
                </Button>
              </div>
            )}
          </div>

          {isLoading && (
            <div className="flex h-48 items-center justify-center">
              <Spinner delay={300} />
            </div>
          )}

          {isError && !isLoading && (
            <p className="text-body-sm text-muted-foreground">
              Dataset not available.
            </p>
          )}

          {data && (
            <DatasetView
              deploymentId={deploymentId}
              account={account}
              summary={data}
            />
          )}
        </div>
      </div>
    </div>
  );
}
