import { ArrowLeft, ExternalLink, ShieldCheck } from "lucide-react";
import { Link, useParams } from "react-router";

import { useAuthorizationResources } from "@/api/admin";
import { DeploymentAccessPanel } from "@/components/deployment-access-panel";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn, formatDateTime } from "@/lib/utils";

export function ResourceDetailPage() {
  const { type = "", id = "" } = useParams<{ type: string; id: string }>();
  const resourcesQuery = useAuthorizationResources();
  const resource = resourcesQuery.data?.resources?.find(
    (candidate) => candidate.type === type && candidate.external_id === id,
  );

  if (resourcesQuery.isLoading) return <Skeleton className="h-72 w-full" />;
  if (resourcesQuery.error) {
    return <p className="text-sm text-destructive">Resource could not be loaded: {resourcesQuery.error.message}</p>;
  }
  if (!resource) {
    return (
      <div className="space-y-3">
        <Button asChild variant="ghost" size="sm"><Link to="/admin/resources"><ArrowLeft className="size-3.5" /> Resources</Link></Button>
        <p className="text-sm text-muted-foreground">This resource is no longer present in WorkOS.</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-2 mb-2">
          <Link to="/admin/resources"><ArrowLeft className="size-3.5" /> Resources</Link>
        </Button>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div className="flex items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-md bg-honey/10 text-honey-dark">
                <ShieldCheck className="size-4" />
              </span>
              <div>
                <h2 className="text-xl font-semibold">{resource.name || "Unnamed resource"}</h2>
                <p className="text-xs uppercase tracking-wide text-muted-foreground">{resource.type}</p>
              </div>
            </div>
          </div>
          {resource.type === "deployment" && (
            <Button asChild variant="outline" size="sm">
              <Link to={`/admin/deployments/${encodeURIComponent(resource.external_id)}`}>
                Open deployment <ExternalLink className="size-3.5" />
              </Link>
            </Button>
          )}
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
        <Metadata label="Astro external ID" value={resource.external_id} mono />
        <Metadata label="WorkOS resource ID" value={resource.workos_resource_id} mono />
        <Metadata label="Account" value={resource.account_name || resource.account_id || "Unresolved"} />
        <Metadata label="Created" value={formatDateTime(resource.created_at)} />
      </div>

      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className={cn(
          "rounded-full px-2 py-1 font-medium",
          resource.last_error
            ? "bg-destructive/10 text-destructive"
            : "bg-green-500/10 text-green-700 dark:text-green-400",
        )}>
          {resource.last_error ? "Sync error" : resource.sync_state}
        </span>
        <span className="text-muted-foreground">{resource.assignment_count} direct assignments</span>
        {resource.last_error && <span className="text-destructive">{resource.last_error}</span>}
      </div>

      {resource.type === "deployment" ? (
        <DeploymentAccessPanel deploymentId={resource.external_id} />
      ) : (
        <div className="rounded-lg glass px-4 py-10 text-center text-sm text-muted-foreground">
          Access evidence for {resource.type} resources will appear here when that resource type is registered.
        </div>
      )}
    </div>
  );
}

function Metadata({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg glass px-3 py-2.5">
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className={cn("mt-1 break-all text-xs font-medium", mono && "font-mono")}>{value}</div>
    </div>
  );
}
