import { Link } from "react-router";
import { useDeployments } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime, truncateUUID } from "@/lib/utils";

export function DeploymentsPage() {
  const { data, isLoading, error } = useDeployments();

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Deployments</h2>
      {isLoading && <TableSkeleton />}
      {error && <p className="text-destructive">Error: {error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Namespace</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Status</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Account</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Build</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Created</th>
              </tr>
            </thead>
            <tbody>
              {data.deployments?.map((d) => (
                <tr key={d.namespace} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-4 py-2">
                    <Link to={`/admin/deployments/${d.namespace}`} className="text-amber hover:underline">
                      {d.name}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-muted-foreground">{d.namespace}</td>
                  <td className="px-4 py-2">
                    <StatusBadge status={d.status} />
                  </td>
                  <td className="px-4 py-2 text-muted-foreground">{d.account_name}</td>
                  <td className="px-4 py-2 font-mono text-xs text-muted-foreground">{truncateUUID(d.build_id)}</td>
                  <td className="px-4 py-2 text-muted-foreground">{formatDateTime(d.created_at)}</td>
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
    running: "bg-green-100/60 backdrop-blur-sm text-green-700",
    pending: "bg-yellow-100/60 backdrop-blur-sm text-yellow-700",
    failed: "bg-red-100/60 backdrop-blur-sm text-red-700",
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
