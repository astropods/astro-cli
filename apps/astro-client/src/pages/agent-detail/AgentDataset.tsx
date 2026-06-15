import { Download } from "lucide-react";
import { useAgentDetailContext } from "../AgentDetail";
import { useEvalDataset } from "@/api/queries/deployments";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

export default function AgentDataset() {
  const { deploymentId } = useAgentDetailContext();
  const { data, isLoading, isError } = useEvalDataset(deploymentId);

  const downloadUrl = `/api/v1/deployments/${encodeURIComponent(deploymentId)}/dataset/download`;

  return (
    <div className="relative z-10 flex flex-1 flex-col pt-16">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-4xl px-6 py-8 pb-16">
          <div className="mb-6 flex items-center justify-between">
            <h2 className="text-heading-4 text-foreground">Dataset</h2>
            {data && (
              <Button asChild variant="outline" size="sm">
                <a href={downloadUrl} download>
                  <Download className="size-4" />
                  Download
                </a>
              </Button>
            )}
          </div>

          {isLoading && (
            <div className="flex h-48 items-center justify-center">
              <Spinner delay={300} />
            </div>
          )}

          {isError && (
            <p className="text-body-sm text-muted-foreground">
              Dataset not available.
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
            </dl>
          )}
        </div>
      </div>
    </div>
  );
}
