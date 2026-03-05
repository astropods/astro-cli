import { useGrants } from "@/api/openmeter";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime } from "@/lib/utils";

export function GrantsPage() {
  const { data, isLoading, error } = useGrants();

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Grants</h2>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">ID</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Entitlement</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Amount</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Priority</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Effective At</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Expires At</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Rollover (max/min)</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Recurrence</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Voided</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
              </tr>
            </thead>
            <tbody>
              {data.map((g) => (
                <tr key={g.id} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-0.5 font-mono text-xs text-amber">{g.id}</td>
                  <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{g.entitlementId}</td>
                  <td className="px-2 py-0.5 font-medium">{g.amount}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{g.priority ?? "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{formatDateTime(g.effectiveAt)}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{g.expiresAt ? formatDateTime(g.expiresAt) : "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">
                    {g.maxRolloverAmount != null || g.minRolloverAmount != null
                      ? `${g.maxRolloverAmount ?? 0} / ${g.minRolloverAmount ?? 0}`
                      : "-"}
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">{g.recurrence?.interval || "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{g.voidedAt ? formatDateTime(g.voidedAt) : "No"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{g.createdAt ? formatDateTime(g.createdAt) : "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {data?.length === 0 && <p className="text-sm text-muted-foreground">No grants found.</p>}
    </div>
  );
}
