import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import {
  useAccounts,
  useClusters,
  useRenameAccount,
  useSetAccountCluster,
  useInvalidateAccountCaches,
  useInvalidateAllCaches,
} from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Pencil, Check, X, CircleCheck, CircleX, ChevronLeft, ChevronRight, Trash2, AlertTriangle } from "lucide-react";
import { formatDateTime, truncateUUID } from "@/lib/utils";
import type { AdminAccount, RegisteredCluster } from "@/types/admin";

type StatusFilter = "all" | "active" | "deleted";
type IntegrationFilter = "all" | "langfuse" | "no-langfuse";

const PAGE_SIZE = 25;
const PRIMARY_CLUSTER_VALUE = "__primary__";
const CLUSTER_MSG_TTL_MS = 8000;

function clusterIdFromSelect(value: string): string {
  return value === PRIMARY_CLUSTER_VALUE ? "" : value;
}

function clusterSelectValue(clusterId: string): string {
  return clusterId === "" ? PRIMARY_CLUSTER_VALUE : clusterId;
}

function savedClusterId(account: AdminAccount): string {
  return account.cluster_id ?? "";
}

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
    if (integration === "langfuse" && !a.has_langfuse) return false;
    if (integration === "no-langfuse" && a.has_langfuse) return false;
    return true;
  });
}

export function AccountsPage() {
  const [searchParams] = useSearchParams();
  const initialQuery = searchParams.get("q") ?? "";
  const { data, isLoading, error } = useAccounts();
  const { data: clustersData } = useClusters(true);
  const renameMut = useRenameAccount();
  const setClusterMut = useSetAccountCluster();
  const invalidateAccountMut = useInvalidateAccountCaches();
  const invalidateAllMut = useInvalidateAllCaches();
  // Two-step confirm for the failsafe — first click arms, second click fires.
  const [bustAllArmed, setBustAllArmed] = useState(false);
  const [bustAllResult, setBustAllResult] = useState<string | null>(null);
  const [clusterChangeMsgs, setClusterChangeMsgs] = useState<Record<string, string>>({});
  const [pendingClusterIds, setPendingClusterIds] = useState<Record<string, string>>({});
  const clusterMsgTimeouts = useRef<Record<string, number>>({});
  const additionalClusters = (clustersData?.clusters ?? []).filter((c) => !c.is_primary);
  const [editing, setEditing] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [search, setSearch] = useState(initialQuery);
  const [status, setStatus] = useState<StatusFilter>("active");
  const [integration, setIntegration] = useState<IntegrationFilter>("all");
  const [page, setPage] = useState(0);

  const accounts = data?.accounts ?? [];
  const filtered = useMemo(
    () => filterAccounts(accounts, search, status, integration),
    [accounts, search, status, integration],
  );

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages - 1);
  const pageAccounts = filtered.slice(safePage * PAGE_SIZE, (safePage + 1) * PAGE_SIZE);

  const clearAccountClusterMessage = useCallback((accountId: string) => {
    const existing = clusterMsgTimeouts.current[accountId];
    if (existing !== undefined) {
      window.clearTimeout(existing);
      delete clusterMsgTimeouts.current[accountId];
    }
    setClusterChangeMsgs((prev) => {
      if (!(accountId in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[accountId];
      return next;
    });
  }, []);

  const setAccountClusterMessage = useCallback(
    (accountId: string, message: string) => {
      clearAccountClusterMessage(accountId);
      setClusterChangeMsgs((prev) => ({ ...prev, [accountId]: message }));
      clusterMsgTimeouts.current[accountId] = window.setTimeout(() => {
        clearAccountClusterMessage(accountId);
      }, CLUSTER_MSG_TTL_MS);
    },
    [clearAccountClusterMessage],
  );

  const setPendingCluster = useCallback((accountId: string, clusterId: string) => {
    clearAccountClusterMessage(accountId);
    setPendingClusterIds((prev) => ({ ...prev, [accountId]: clusterId }));
  }, [clearAccountClusterMessage]);

  const clearPendingCluster = useCallback((accountId: string) => {
    setPendingClusterIds((prev) => {
      if (!(accountId in prev)) {
        return prev;
      }
      const next = { ...prev };
      delete next[accountId];
      return next;
    });
  }, []);

  const applyClusterMigration = useCallback(
    (accountId: string, clusterId: string) => {
      setClusterMut.mutate(
        { id: accountId, clusterId },
        {
          onSuccess: (resp) => {
            clearPendingCluster(accountId);
            const count = resp.migrations_enqueued ?? 0;
            if (count > 0) {
              setAccountClusterMessage(
                accountId,
                `${count} deployment migration${count === 1 ? "" : "s"} queued. Track in Admin → Migrations.`,
              );
            } else {
              setAccountClusterMessage(
                accountId,
                "Account cluster updated; no deployment migrations queued. If routing should have moved, check Admin → Migrations.",
              );
            }
          },
          onError: (e) => {
            setAccountClusterMessage(accountId, `Cluster change failed: ${(e as Error).message}`);
          },
        },
      );
    },
    [clearPendingCluster, setAccountClusterMessage, setClusterMut],
  );

  useEffect(() => {
    const timeouts = clusterMsgTimeouts.current;
    return () => {
      Object.values(timeouts).forEach((id) => window.clearTimeout(id));
    };
  }, []);

  // Reset to page 0 when filters change
  const updateFilter = <T,>(setter: React.Dispatch<React.SetStateAction<T>>) => (v: T) => {
    setter(v);
    setPage(0);
  };

  const onInvalidateAccount = (id: string, name: string) => {
    if (!window.confirm(`Invalidate agents-page caches for "${name}"?`)) return;
    invalidateAccountMut.mutate(id);
  };

  const onBustAllClick = () => {
    if (!bustAllArmed) {
      setBustAllArmed(true);
      // Disarm after a few seconds so a stray armed state can't sit forever.
      window.setTimeout(() => setBustAllArmed(false), 5000);
      return;
    }
    setBustAllArmed(false);
    invalidateAllMut.mutate(undefined, {
      onSuccess: (r) =>
        setBustAllResult(`Busted ${r.accounts_busted} account(s), ${r.deployments_busted} deployment(s)`),
      onError: (e) => setBustAllResult(`Failed: ${(e as Error).message}`),
    });
  };

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-xl font-semibold">Accounts</h2>
        <div className="flex items-center gap-2">
          {bustAllResult && (
            <span className="text-xs text-muted-foreground">{bustAllResult}</span>
          )}
          <Button
            variant={bustAllArmed ? "destructive" : "outline"}
            size="sm"
            onClick={onBustAllClick}
            disabled={invalidateAllMut.isPending}
            title="Failsafe — clears the agents-page cache (deploy envelope + obs summaries) for every account. Click twice within 5s to confirm."
          >
            <AlertTriangle className="size-3.5" />
            {invalidateAllMut.isPending
              ? "Busting…"
              : bustAllArmed
                ? "Click again to confirm"
                : "Invalidate all caches"}
          </Button>
        </div>
      </div>

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
            <SelectItem value="langfuse">Has Langfuse</SelectItem>
            <SelectItem value="no-langfuse">No Langfuse</SelectItem>
          </SelectContent>
        </Select>
        <span className="ml-auto text-xs text-muted-foreground">
          {filtered.length} account{filtered.length !== 1 ? "s" : ""}
          {accounts.length > 0 && filtered.length !== accounts.length && ` of ${accounts.length}`}
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
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Cluster</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Owner</th>
                  <th className="px-2 py-0.5 text-right font-medium text-muted-foreground">Members</th>
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
                      <td className="px-2 py-0.5">
                        {isDeleted ? (
                          <span className="text-muted-foreground">
                            {a.cluster_id || "primary"}
                          </span>
                        ) : (
                          <AccountClusterCell
                            account={a}
                            additionalClusters={additionalClusters}
                            pendingClusterId={pendingClusterIds[a.id]}
                            message={clusterChangeMsgs[a.id]}
                            isMigrating={setClusterMut.isPending && setClusterMut.variables?.id === a.id}
                            onPendingChange={setPendingCluster}
                            onClearPending={clearPendingCluster}
                            onMigrate={applyClusterMigration}
                          />
                        )}
                      </td>
                      <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{a.owner_user_id ? truncateUUID(a.owner_user_id) : "-"}</td>
                      <td className="px-2 py-0.5 text-right">{a.member_count}</td>
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
                          <div className="flex items-center gap-1">
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => { setEditing(a.id); setNewName(a.name); }}
                              title="Rename"
                            >
                              <Pencil className="size-3" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              onClick={() => onInvalidateAccount(a.id, a.name)}
                              disabled={invalidateAccountMut.isPending}
                              title="Invalidate agents-page caches for this account"
                            >
                              <Trash2 className="size-3" />
                            </Button>
                          </div>
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

type AccountClusterCellProps = {
  account: AdminAccount;
  additionalClusters: RegisteredCluster[];
  pendingClusterId: string | undefined;
  message: string | undefined;
  isMigrating: boolean;
  onPendingChange: (accountId: string, clusterId: string) => void;
  onClearPending: (accountId: string) => void;
  onMigrate: (accountId: string, clusterId: string) => void;
};

function AccountClusterCell({
  account,
  additionalClusters,
  pendingClusterId,
  message,
  isMigrating,
  onPendingChange,
  onClearPending,
  onMigrate,
}: AccountClusterCellProps) {
  const savedId = savedClusterId(account);
  const effectiveId = pendingClusterId ?? savedId;
  const hasPendingChange = effectiveId !== savedId;

  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex items-center gap-1">
        <Select
          value={clusterSelectValue(effectiveId)}
          onValueChange={(v) => onPendingChange(account.id, clusterIdFromSelect(v))}
          disabled={isMigrating}
        >
          <SelectTrigger className="h-7 w-44 text-xs">
            <SelectValue placeholder="Primary (default)" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={PRIMARY_CLUSTER_VALUE}>Primary (default)</SelectItem>
            {additionalClusters.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.id}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {hasPendingChange && (
          <>
            <Button
              variant="default"
              size="sm"
              className="h-7 px-2 text-xs"
              disabled={isMigrating}
              onClick={() => onMigrate(account.id, effectiveId)}
            >
              {isMigrating ? "Migrating…" : "Migrate"}
            </Button>
            <Button
              variant="ghost"
              size="icon-xs"
              disabled={isMigrating}
              onClick={() => onClearPending(account.id)}
              title="Cancel cluster change"
            >
              <X className="size-3" />
            </Button>
          </>
        )}
      </div>
      {message && (
        <span className="max-w-56 text-[10px] leading-tight text-muted-foreground">{message}</span>
      )}
    </div>
  );
}
