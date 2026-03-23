import { useMemo, useState } from "react";
import { useAccounts, useRenameAccount } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Pencil, Check, X, CircleCheck, CircleX, ChevronLeft, ChevronRight } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import type { AdminAccount } from "@/types/admin";

type StatusFilter = "all" | "active" | "deleted";
type IntegrationFilter = "all" | "openmeter" | "no-openmeter" | "langfuse" | "no-langfuse";

const PAGE_SIZE = 25;

function filterAccounts(
  accounts: AdminAccount[],
  search: string,
  status: StatusFilter,
  integration: IntegrationFilter,
): AdminAccount[] {
  return accounts.filter((a) => {
    if (search) {
      const q = search.toLowerCase();
      if (!a.name.toLowerCase().includes(q) && !a.id.toLowerCase().includes(q)) return false;
    }
    if (status === "active" && a.deleted_at) return false;
    if (status === "deleted" && !a.deleted_at) return false;
    if (integration === "openmeter" && !a.has_openmeter) return false;
    if (integration === "no-openmeter" && a.has_openmeter) return false;
    if (integration === "langfuse" && !a.has_langfuse) return false;
    if (integration === "no-langfuse" && a.has_langfuse) return false;
    return true;
  });
}

export function AccountsPage() {
  const { data, isLoading, error } = useAccounts();
  const renameMut = useRenameAccount();
  const [editing, setEditing] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>("active");
  const [integration, setIntegration] = useState<IntegrationFilter>("all");
  const [page, setPage] = useState(0);

  const filtered = useMemo(
    () => (data?.accounts ? filterAccounts(data.accounts, search, status, integration) : []),
    [data?.accounts, search, status, integration],
  );

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages - 1);
  const pageAccounts = filtered.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);

  // Reset to page 0 when filters change
  const updateFilter = <T,>(setter: React.Dispatch<React.SetStateAction<T>>) => (v: T) => {
    setter(v);
    setPage(0);
  };

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Accounts</h2>

      {/* Filters */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Input
          placeholder="Search name or ID..."
          value={search}
          onChange={(e) => updateFilter(setSearch)(e.target.value)}
          className="h-7 w-56 text-xs"
        />
        <Select value={status} onValueChange={updateFilter(setStatus)}>
          <SelectTrigger className="h-7 w-28 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All</SelectItem>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="deleted">Deleted</SelectItem>
          </SelectContent>
        </Select>
        <Select value={integration} onValueChange={updateFilter(setIntegration)}>
          <SelectTrigger className="h-7 w-40 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All integrations</SelectItem>
            <SelectItem value="openmeter">Has OpenMeter</SelectItem>
            <SelectItem value="no-openmeter">No OpenMeter</SelectItem>
            <SelectItem value="langfuse">Has Langfuse</SelectItem>
            <SelectItem value="no-langfuse">No Langfuse</SelectItem>
          </SelectContent>
        </Select>
        <span className="ml-auto text-xs text-muted-foreground">
          {filtered.length} account{filtered.length !== 1 ? "s" : ""}
          {data?.accounts && filtered.length !== data.accounts.length && ` of ${data.accounts.length}`}
        </span>
      </div>

      {isLoading && <LoadingSkeleton />}
      {error && <p className="text-destructive">Error: {error.message}</p>}
      {data && (
        <>
          <div className="overflow-x-auto rounded-lg glass">
            <table className="w-full text-[11px] whitespace-nowrap">
              <thead>
                <tr className="border-b border-glass-border-honey glass-subtle">
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Type</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Owner</th>
                  <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Members</th>
                  <th className="px-2 py-0.5 text-center font-medium text-muted-foreground">OpenMeter</th>
                  <th className="px-2 py-0.5 text-center font-medium text-muted-foreground">Langfuse</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Status</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Actions</th>
                </tr>
              </thead>
              <tbody>
                {pageAccounts.map((a) => {
                  const isDeleted = !!a.deleted_at;
                  return (
                    <tr
                      key={a.id}
                      className={`border-b border-comb-light hover:bg-glass-light ${isDeleted ? "opacity-50" : ""}`}
                    >
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
                      <td className="px-2 py-0.5 text-center">
                        {a.has_openmeter ? (
                          <CircleCheck className="inline size-3.5 text-green-500" />
                        ) : (
                          <CircleX className="inline size-3.5 text-muted-foreground/40" />
                        )}
                      </td>
                      <td className="px-2 py-0.5 text-center">
                        {a.has_langfuse ? (
                          <CircleCheck className="inline size-3.5 text-green-500" />
                        ) : (
                          <CircleX className="inline size-3.5 text-muted-foreground/40" />
                        )}
                      </td>
                      <td className="px-2 py-0.5">
                        {isDeleted ? (
                          <span className="inline-flex items-center rounded-full bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium text-destructive">
                            Deleted {formatDateTime(a.deleted_at!)}
                          </span>
                        ) : (
                          <span className="inline-flex items-center rounded-full bg-green-500/10 px-1.5 py-0.5 text-[10px] font-medium text-green-600">
                            Active
                          </span>
                        )}
                      </td>
                      <td className="px-2 py-0.5 text-muted-foreground">{formatDateTime(a.created_at)}</td>
                      <td className="px-2 py-0.5">
                        {editing !== a.id && !isDeleted && (
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
                  );
                })}
                {pageAccounts.length === 0 && (
                  <tr>
                    <td colSpan={9} className="px-2 py-4 text-center text-muted-foreground">
                      No accounts match the current filters.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
              <span>
                Page {safePage + 1} of {totalPages}
              </span>
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  disabled={safePage === 0}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  disabled={safePage >= totalPages - 1}
                  onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
                >
                  <ChevronRight className="size-3.5" />
                </Button>
              </div>
            </div>
          )}
        </>
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
