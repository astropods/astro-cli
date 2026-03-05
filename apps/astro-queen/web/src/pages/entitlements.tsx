import { Link } from "react-router";
import { useEntitlements } from "@/api/openmeter";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime } from "@/lib/utils";

export function EntitlementsPage() {
  const { data, isLoading, error } = useEntitlements();

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Entitlements</h2>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">ID</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Feature</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Type</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Customer</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Soft Limit</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Usage Period</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Active From</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
              </tr>
            </thead>
            <tbody>
              {data.map((ent) => (
                <tr key={ent.id} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-0.5 font-mono text-xs text-amber">{ent.id}</td>
                  <td className="px-2 py-0.5">
                    <span className="font-medium">{ent.featureKey}</span>
                  </td>
                  <td className="px-2 py-0.5">
                    <span className={
                      ent.type === "metered" ? "text-blue-500" :
                      ent.type === "boolean" ? "text-green-600" :
                      "text-purple-500"
                    }>
                      {ent.type}
                    </span>
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">
                    {ent.customerId ? (
                      <Link to={`/openmeter/customers/${ent.customerId}`} className="text-amber hover:underline">
                        {ent.customerId}
                      </Link>
                    ) : ent.subjectKey || "-"}
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">{ent.isSoftLimit ? "Yes" : "No"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{ent.usagePeriod?.interval || "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{ent.activeFrom ? formatDateTime(ent.activeFrom) : "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{ent.createdAt ? formatDateTime(ent.createdAt) : "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {data?.length === 0 && <p className="text-sm text-muted-foreground">No entitlements found.</p>}
    </div>
  );
}
