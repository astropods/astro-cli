import { useState } from "react";
import { Link } from "react-router";
import { useDeployments, useBackfillDeployments } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import { DatabaseZap } from "lucide-react";

export function DeploymentsPage() {
  const { data, isLoading, error } = useDeployments();
  const backfillMut = useBackfillDeployments();
  const [backfillResult, setBackfillResult] = useState<string | null>(null);

  const hasLegacy = data?.deployments?.some((d) => d.current_revision == null);

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-xl font-semibold">Deployments</h2>
        {hasLegacy && (
          <Button
            variant="outline"
            size="sm"
            disabled={backfillMut.isPending}
            onClick={() => {
              backfillMut.mutate(undefined, {
                onSuccess: (data) => {
                  setBackfillResult(`Backfilled ${(data as { backfilled_count: number }).backfilled_count} deployments`);
                  setTimeout(() => setBackfillResult(null), 5000);
                },
              });
            }}
          >
            <DatabaseZap className="size-3.5" />
            Backfill Revisions
          </Button>
        )}
      </div>
      {backfillResult && (
        <p className="mb-3 text-sm text-green-600">{backfillResult}</p>
      )}
      {isLoading && <TableSkeleton />}
      {error && <p className="text-destructive">Error: {error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Namespace</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Status</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Rev</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Account</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Build</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Error</th>
              </tr>
            </thead>
            <tbody>
              {data.deployments?.map((d) => (
                <tr key={d.deployment_id} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-0.5">
                    <Link to={`/admin/deployments/${d.deployment_id}`} className="text-amber hover:underline">
                      {d.name}
                    </Link>
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">{d.namespace}</td>
                  <td className="px-2 py-0.5">
                    <StatusBadge status={d.status} />
                    {d.status_changed_at && (
                      <span className="ml-1 text-[10px] text-muted-foreground" title={d.status_changed_at}>
                        {formatDistanceToNow(new Date(d.status_changed_at), { addSuffix: true })}
                      </span>
                    )}
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground font-mono">
                    {d.current_revision != null ? `rev ${d.current_revision}` : "-"}
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">{d.account_name}</td>
                  <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{truncateUUID(d.build_id)}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{formatDateTime(d.created_at)}</td>
                  <td className="max-w-[200px] truncate px-2 py-0.5 text-muted-foreground" title={d.error_message}>
                    {d.error_message || ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    active: "bg-green-100/60 backdrop-blur-sm text-green-700",
    running: "bg-green-100/60 backdrop-blur-sm text-green-700",
    pending: "bg-yellow-100/60 backdrop-blur-sm text-yellow-700",
    provisioning: "bg-blue-100/60 backdrop-blur-sm text-blue-700",
    failed: "bg-red-100/60 backdrop-blur-sm text-red-700",
    undeploying: "bg-orange-100/60 backdrop-blur-sm text-orange-700",
    scaled_down: "bg-purple-100/60 backdrop-blur-sm text-purple-700",
  };
  return (
    <span className={`inline-block rounded-full px-2 py-0.5 text-xs ${colors[status?.toLowerCase()] ?? "rounded-full bg-pollen-light text-honey-dark"}`}>
      {status || "unknown"}
    </span>
  );
}

function TableSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
