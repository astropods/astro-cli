import { useState } from "react";
import { useAccounts, useRenameAccount } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Pencil, Check, X } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";

export function AccountsPage() {
  const { data, isLoading, error } = useAccounts();
  const renameMut = useRenameAccount();
  const [editing, setEditing] = useState<string | null>(null);
  const [newName, setNewName] = useState("");

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Accounts</h2>
      {isLoading && <LoadingSkeleton />}
      {error && <p className="text-red-400">Error: {error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-md border border-stone-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-stone-800 bg-stone-900/50">
                <th className="px-4 py-2 text-left font-medium text-stone-400">Name</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Type</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Owner</th>
                <th className="px-4 py-2 text-right font-medium text-stone-400">Members</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Created</th>
                <th className="px-4 py-2 text-left font-medium text-stone-400">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.accounts?.map((a) => (
                <tr key={a.id} className="border-b border-stone-800/50 hover:bg-stone-900/30">
                  <td className="px-4 py-2">
                    {editing === a.id ? (
                      <div className="flex items-center gap-1">
                        <Input
                          value={newName}
                          onChange={(e) => setNewName(e.target.value)}
                          className="h-7 w-48"
                          autoFocus
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              renameMut.mutate({ id: a.id, newName }, { onSuccess: () => setEditing(null) });
                            }
                            if (e.key === "Escape") setEditing(null);
                          }}
                        />
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={() => renameMut.mutate({ id: a.id, newName }, { onSuccess: () => setEditing(null) })}
                        >
                          <Check className="size-3" />
                        </Button>
                        <Button variant="ghost" size="icon-xs" onClick={() => setEditing(null)}>
                          <X className="size-3" />
                        </Button>
                      </div>
                    ) : (
                      a.name
                    )}
                  </td>
                  <td className="px-4 py-2 text-stone-400">{a.type}</td>
                  <td className="px-4 py-2 font-mono text-xs text-stone-500">{truncateUUID(a.owner_user_id)}</td>
                  <td className="px-4 py-2 text-right">{a.member_count}</td>
                  <td className="px-4 py-2 text-stone-500">{formatDateTime(a.created_at)}</td>
                  <td className="px-4 py-2">
                    {editing !== a.id && (
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => { setEditing(a.id); setNewName(a.name); }}
                      >
                        <Pencil className="size-3" />
                      </Button>
                    )}
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

function LoadingSkeleton() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
