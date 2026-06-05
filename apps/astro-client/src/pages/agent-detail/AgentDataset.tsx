import { Check, Download, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { useAgentDetailContext } from "../AgentDetail";
import { useEvalDataset, useTriggerEvalDatasetSync } from "@/api/queries/deployments";
import { formatTimestamp } from "@/components/agent-detail/traces/trace-utils";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

export default function AgentDataset() {
  const { deploymentId } = useAgentDetailContext();
  const { data, isLoading, isError } = useEvalDataset(deploymentId);
  const sync = useTriggerEvalDatasetSync(deploymentId);
  const [showScheduled, setShowScheduled] = useState(false);

  useEffect(() => {
    if (sync.isSuccess) {
      setShowScheduled(true);
      const t = setTimeout(() => setShowScheduled(false), 1000);
      return () => clearTimeout(t);
    }
  }, [sync.isSuccess]);

  const downloadUrl = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/download`;

  return (
    <div className="relative z-10 flex flex-1 flex-col pt-16">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-4xl px-6 py-8 pb-16">
          <div className="mb-6 flex items-center justify-between">
            <h2 className="text-heading-4 text-foreground">Dataset</h2>
            {data && (
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => sync.mutate()}
                  disabled={sync.isPending || showScheduled}
                >
                  {showScheduled ? (
                    <>
                      <Check className="size-4" />
                      Scheduled
                    </>
                  ) : (
                    <>
                      <RefreshCw className={`size-4 ${sync.isPending ? "animate-spin" : ""}`} />
                      Sync
                    </>
                  )}
                </Button>
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

          {isError && (
            <p className="text-body-sm text-muted-foreground">
              Dataset not available. Datasets are created for running deployments daily.
            </p>
          )}

          {data && (
            <dl className="flex flex-col gap-4">
              <div className="flex items-baseline gap-2">
                <Label size="md" className="mb-0">Name</Label>
                <dd className="font-mono text-[13px] text-muted-foreground">{data.dataset_name}</dd>
              </div>
              <div className="flex items-baseline gap-2">
                <Label size="md" className="mb-0">Item count</Label>
                <dd className="text-[13px] text-muted-foreground">{data.item_count.toLocaleString()}</dd>
              </div>
              <div className="flex items-baseline gap-2">
                <Label size="md" className="mb-0">Last synced</Label>
                <dd className="text-[13px] text-muted-foreground">
                  {data.last_synced_at ? formatTimestamp(data.last_synced_at) : "Never"}
                </dd>
              </div>
            </dl>
          )}
        </div>
      </div>
    </div>
  );
}
