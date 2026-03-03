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
      {error && <p className="text-red-400 text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-md border border-stone-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-stone-800 bg-stone-900/50">
                <th className="px-4 py-2 text-left font-medium text-stone-400">ID</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Name</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Email</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Currency</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Created</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((c) => (
                <tr key={c.id} className="border-b border-stone-800/50 hover:bg-stone-900/30">
                  <td className="px-4 py-2">
                    <Link to={`/openmeter/customers/${c.id}`} className="text-amber hover:underline">
                      {c.id}
                    </Link>
                  </td>
                  <td className="px-4 py-2">{c.name}</td>
                  <td className="px-4 py-2 text-stone-400">{c.email || "-"}</td>
                  <td className="px-4 py-2 text-stone-400">{c.currency || "-"}</td>
                  <td className="px-4 py-2 text-stone-500">{c.createdAt ? formatDateTime(c.createdAt) : "-"}</td>
                  <td className="px-4 py-2">
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={() => { if (confirm(`Delete customer "${c.name}"?`)) deleteMut.mutate(c.id); }}
                    >
                      <Trash2 className="size-3 text-red-400" />
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
