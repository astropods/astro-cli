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
      {error && <p className="text-destructive">Error: {error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px] whitespace-nowrap">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Type</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Owner</th>
                <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Members</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.accounts?.map((a) => (
                <tr key={a.id} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-2 py-0.5">
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
                  <td className="px-2 py-0.5 text-muted-foreground">{a.type}</td>
                  <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{truncateUUID(a.owner_user_id)}</td>
                  <td className="px-2 py-0.5 text-right">{a.member_count}</td>
                  <td className="px-2 py-0.5 text-muted-foreground">{formatDateTime(a.created_at)}</td>
                  <td className="px-2 py-0.5">
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
