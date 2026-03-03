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
      {error && <p className="text-red-400">Error: {error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-md border border-stone-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-stone-800 bg-stone-900/50">
                <th className="px-4 py-2 text-left font-medium text-stone-400">Name</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Namespace</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Status</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Account</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Build</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Created</th>
              </tr>
            </thead>
            <tbody>
              {data.deployments?.map((d) => (
                <tr key={d.namespace} className="border-b border-stone-800/50 hover:bg-stone-900/30">
                  <td className="px-4 py-2">
                    <Link to={`/admin/deployments/${d.namespace}`} className="text-amber hover:underline">
                      {d.name}
                    </Link>
                  </td>
                  <td className="px-4 py-2 text-stone-400">{d.namespace}</td>
                  <td className="px-4 py-2">
                    <StatusBadge status={d.status} />
                  </td>
                  <td className="px-4 py-2 text-stone-400">{d.account_name}</td>
                  <td className="px-4 py-2 font-mono text-xs text-stone-500">{truncateUUID(d.build_id)}</td>
                  <td className="px-4 py-2 text-stone-500">{formatDateTime(d.created_at)}</td>
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
    running: "bg-green-900/40 text-green-400",
    pending: "bg-yellow-900/40 text-yellow-400",
    failed: "bg-red-900/40 text-red-400",
  };
  return (
    <span className={`inline-block rounded-full px-2 py-0.5 text-xs ${colors[status?.toLowerCase()] ?? "bg-stone-800 text-stone-400"}`}>
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
