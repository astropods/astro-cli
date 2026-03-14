import { Link } from "react-router";
import { useCustomers, useDeleteCustomer } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Trash2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";

export function CustomersPage() {
  const { data, isLoading, error } = useCustomers();
  const deleteMut = useDeleteCustomer();

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Customers</h2>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">ID</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Email</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Currency</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Subscription</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((c) => (
                <tr key={c.id} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-0.5">
                    <Link to={`/openmeter/customers/${c.id}`} className="text-amber hover:underline">
                      {c.id}
                    </Link>
                  </td>
                  <td className="px-2 py-0.5">{c.name}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{c.primaryEmail || "-"}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{c.currency || "-"}</td>
                  <td className="px-2 py-0.5">
                    {c.currentSubscriptionId
                      ? <span className="text-green-600">active</span>
                      : <span className="text-muted-foreground">none</span>}
                  </td>
                  <td className="px-2 py-0.5 text-muted-foreground">{c.createdAt ? formatDateTime(c.createdAt) : "-"}</td>
                  <td className="px-2 py-0.5">
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => { if (confirm(`Delete customer "${c.name}"?`)) deleteMut.mutate(c.id); }}
                    >
                      <Trash2 className="size-3 text-red-500" />
                    </Button>
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
