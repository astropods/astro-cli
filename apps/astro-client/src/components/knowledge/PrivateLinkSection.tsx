import { ArrowPathIcon, CheckIcon, ExclamationTriangleIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { ErrorPanel } from "@/components/ui/status-panel";
import { useRecheckKnowledgeStore } from "@/api/queries/knowledge";
import { cn } from "@/lib/utils";
import type { KnowledgeStore } from "@/lib/api";

const CLOUD_CONSOLE: Record<string, {
  stepTitle: string;
  label: string;
  description: string;
  url: (region: string, endpointId?: string) => string;
}> = {
  aws: {
    stepTitle: "Approve the VPC Endpoint request",
    label: "Open AWS Console ↗",
    description: "In AWS Console, go to VPC → Endpoints and approve the pending connection from Astro.",
    url: (region, endpointId) =>
      `https://console.aws.amazon.com/vpc/home?region=${region}#Endpoints:${endpointId ? `endpointId=${endpointId}` : ""}`,
  },
  gcp: {
    stepTitle: "Approve the Private Service Connect endpoint",
    label: "Open GCP Console ↗",
    description: "In GCP Console, go to Private Service Connect and approve the pending endpoint request from Astro.",
    url: () => "https://console.cloud.google.com/net-services/psc/list/endpoints",
  },
  azure: {
    stepTitle: "Approve the Private Endpoint connection",
    label: "Open Azure Portal ↗",
    description: "In Azure Portal, go to Private Link Center → Pending connections and approve the request from Astro.",
    url: () => "https://portal.azure.com/#view/HubsExtension/BrowseResource/resourceType/Microsoft.Network%2FprivateEndpoints",
  },
};

export function PrivateLinkSection({
  store,
  account,
  showBanner = true,
}: {
  store: KnowledgeStore;
  account?: string;
  showBanner?: boolean;
}) {
  const recheck = useRecheckKnowledgeStore(account ?? "");

  if (!store.endpoint) return null;

  const cloud = CLOUD_CONSOLE[store.endpoint.cloud_provider] ?? CLOUD_CONSOLE.aws;
  const consoleUrl = cloud.url(store.endpoint.region, store.endpoint.endpoint_id);
  const status = store.endpoint.status;
  const isPending = status === "pending-acceptance";
  const isReady = status === "ready";

  return (
    <div className="space-y-3">
      {/* Action required warning banner */}
      {isPending && showBanner && (
        <div className="flex items-start gap-3 rounded-lg border border-yellow-200 bg-yellow-50 px-4 py-3.5 text-sm text-yellow-800">
          <ExclamationTriangleIcon className="size-4 shrink-0 mt-0.5 text-warning" />
          <div>
            <p className="font-medium">Action required</p>
            <p className="text-warning">Accept the endpoint connection request in your cloud console to complete setup.</p>
          </div>
        </div>
      )}

      {/* Steps card */}
      <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface divide-y divide-border">

        {/* Step 1 — Complete */}
        <div className="flex items-start gap-4 px-5 py-4">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-success/30 bg-success/10 mt-0.5">
            <CheckIcon className="size-3.5 text-success stroke-[2]" />
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <p className="text-body font-medium text-foreground">Store registered in Astro</p>
            {store.endpoint.region && (
              <p className="mt-1 font-mono text-mono-sm text-muted-foreground">
                {[store.endpoint.endpoint_id, store.endpoint.region].filter(Boolean).join(" · ")}
              </p>
            )}
          </div>
        </div>

        {/* Step 2 — Active or complete */}
        <div className="flex items-start gap-4 px-5 py-4">
          <div
            className={cn(
              "flex size-8 shrink-0 items-center justify-center rounded-full border mt-0.5",
              isReady ? "border-success/30 bg-success/10" : "border-warning/30 bg-warning/10"
            )}
          >
            {isReady
              ? <CheckIcon className="size-3.5 text-success stroke-[2]" />
              : <span className="text-body-sm font-semibold text-warning">2</span>
            }
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <p className="text-body font-medium text-foreground">{cloud.stepTitle}</p>
            {isPending && (
              <>
                <p className="mt-1 text-body-sm text-muted-foreground">{cloud.description}</p>
                <Button variant="outline" size="sm" className="mt-3" asChild>
                  <a href={consoleUrl} target="_blank" rel="noreferrer">{cloud.label}</a>
                </Button>
              </>
            )}
          </div>
        </div>

        {/* Step 3 — Locked or complete */}
        <div className="flex items-start gap-4 px-5 py-4">
          <div className={`flex size-8 shrink-0 items-center justify-center rounded-full border mt-0.5 ${isReady ? "border-success/30 bg-success/10" : "border-border"}`}>
            {isReady
              ? <CheckIcon className="size-3.5 text-success stroke-[2]" />
              : <span className="text-body-sm font-medium text-muted-foreground">3</span>
            }
          </div>
          <div className="flex-1 min-w-0 pt-0.5">
            <p className={`text-body font-medium ${isReady ? "text-foreground" : "text-faint-foreground"}`}>
              Astro verifies your connection
            </p>
            <p className={`mt-0.5 text-body-sm ${isReady ? "text-muted-foreground" : "text-faint-foreground"}`}>
              {isReady ? "Connection verified and store is ready." : "Happens automatically after you approve."}
            </p>
          </div>
        </div>
      </div>

      {/* Recheck action — re-resolves the endpoint and repairs the stored host.
          Useful for stores connected before host auto-resolution, or to re-poll
          a pending endpoint after approving it in the cloud console. */}
      {account && (
        <div className="space-y-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => recheck.mutate({ name: store.name })}
            disabled={recheck.isPending}
          >
            {recheck.isPending
              ? <Spinner size={14} className="mr-2" />
              : <ArrowPathIcon className="size-3.5 mr-1.5" />}
            Recheck connection
          </Button>
          {recheck.isError && (
            <ErrorPanel variant="inline">
              {recheck.error instanceof Error ? recheck.error.message : "Recheck failed"}
            </ErrorPanel>
          )}
        </div>
      )}

    </div>
  );
}
