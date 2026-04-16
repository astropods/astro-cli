import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import type { Route } from "./+types/KnowledgeStores";
import { PlusIcon, LinkIcon } from "@heroicons/react/24/outline";
import { EllipsisHorizontalIcon } from "@heroicons/react/24/outline";
import { CircleStackIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusBadge } from "@/components/StatusBadge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { OrgSwitcher } from "@/components/OrgSwitcher";
import { useAuth } from "@/lib/auth";
import { useDefaultAccount } from "@/hooks/use-default-account";
import { useKnowledgeStores } from "@/api/queries/knowledge";
import { CreateKnowledgeStoreDialog } from "@/components/knowledge/CreateKnowledgeStoreDialog";
import { ConnectKnowledgeStoreDialog } from "@/components/knowledge/ConnectKnowledgeStoreDialog";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeStore } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Stores | Astro" }];

function formatRelativeTime(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diffMs = now - then;
  const diffMin = Math.floor(diffMs / 60000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ago`;
}

function KnowledgeStoresContent() {
  const { personalAccount, isAuthenticated } = useAuth();
  const { defaultAccount, validStoredDefault, handleSetDefault } = useDefaultAccount();
  const [searchParams, setSearchParams] = useSearchParams();
  const userAccount = searchParams.get("account") || validStoredDefault || personalAccount?.name || "";

  const setActiveAccount = (account: string) => {
    setSearchParams(account === personalAccount?.name ? {} : { account });
  };

  const { data, isLoading } = useKnowledgeStores(userAccount, isAuthenticated);
  const stores = data?.stores ?? [];

  const [createOpen, setCreateOpen] = useState(false);
  const [connectOpen, setConnectOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<KnowledgeStore | null>(null);

  return (
    <div className="flex-1 bg-muted">
      <div className="px-6 py-6">
        <div className="mb-6 flex flex-col gap-3">
          <div className="flex flex-col-reverse gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
            <h1 className="min-w-0 text-heading-1 text-foreground">Knowledge Stores</h1>
            <div className="flex items-center gap-2">
              <OrgSwitcher
                activeAccount={userAccount}
                defaultAccount={defaultAccount}
                onChange={setActiveAccount}
                onSetDefault={handleSetDefault}
              />
              <Button variant="outline" size="sm" onClick={() => setConnectOpen(true)}>
                <LinkIcon className="size-4" />
                Connect External
              </Button>
              <Button size="sm" onClick={() => setCreateOpen(true)}>
                <PlusIcon className="size-4" />
                Create Store
              </Button>
            </div>
          </div>
        </div>

        {isLoading ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-14 w-full rounded-md" />
            ))}
          </div>
        ) : stores.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <CircleStackIcon className="size-12 text-muted-foreground/40 mb-4" />
            <h2 className="text-heading-4 text-foreground mb-1">No knowledge stores yet</h2>
            <p className="text-body-sm text-muted-foreground mb-6 max-w-sm">
              Create one to give your agents a database for memory, vector search, or caching.
            </p>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setConnectOpen(true)}>
                <LinkIcon className="size-4" />
                Connect External
              </Button>
              <Button onClick={() => setCreateOpen(true)}>
                <PlusIcon className="size-4" />
                Create Store
              </Button>
            </div>
          </div>
        ) : (
          <div className="rounded-md border border-border bg-surface overflow-hidden">
            <table className="w-full text-body-sm">
              <thead>
                <tr className="border-b border-border bg-muted/50">
                  <th className="px-4 py-2.5 text-left font-mono text-mono-sm text-muted-foreground">Name</th>
                  <th className="px-4 py-2.5 text-left font-mono text-mono-sm text-muted-foreground">Provider</th>
                  <th className="px-4 py-2.5 text-left font-mono text-mono-sm text-muted-foreground">Mode</th>
                  <th className="px-4 py-2.5 text-left font-mono text-mono-sm text-muted-foreground">Status</th>
                  <th className="px-4 py-2.5 text-left font-mono text-mono-sm text-muted-foreground">Storage</th>
                  <th className="px-4 py-2.5 text-left font-mono text-mono-sm text-muted-foreground">Created</th>
                  <th className="px-4 py-2.5 w-10" />
                </tr>
              </thead>
              <tbody>
                {stores.map((store) => (
                  <tr key={store.id} className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
                    <td className="px-4 py-3">
                      <Link
                        to={knowledgeDetailPath(store.name)}
                        className="font-medium text-foreground hover:text-teal-700 transition-colors"
                      >
                        {store.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {PROVIDER_LABELS[store.provider] ?? store.provider}
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center rounded-full border border-border px-2 py-0.5 font-mono text-mono-sm text-muted-foreground">
                        {store.mode === "managed" ? "Managed" : "External"}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge
                        color={statusToColor(store.status)}
                        indicator
                        spinning={isTransitionalStatus(store.status)}
                      >
                        {statusLabel(store.status)}
                      </StatusBadge>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {store.mode === "managed" ? (store.storage ?? "—") : "—"}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {formatRelativeTime(store.created_at)}
                    </td>
                    <td className="px-4 py-3">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="size-7">
                            <EllipsisHorizontalIcon className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => setDeleteTarget(store)}
                          >
                            Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <CreateKnowledgeStoreDialog account={userAccount} open={createOpen} onOpenChange={setCreateOpen} />
      <ConnectKnowledgeStoreDialog account={userAccount} open={connectOpen} onOpenChange={setConnectOpen} />
      {deleteTarget && (
        <DeleteKnowledgeStoreDialog
          open={!!deleteTarget}
          onOpenChange={(o) => { if (!o) setDeleteTarget(null); }}
          storeName={deleteTarget.name}
          account={userAccount}
          onDeleted={() => setDeleteTarget(null)}
        />
      )}
    </div>
  );
}

export default function KnowledgeStores() {
  return (
    <ProtectedRoute>
      <KnowledgeStoresContent />
    </ProtectedRoute>
  );
}
