import { useState } from "react";
import { useNavigate } from "react-router";
import type { Route } from "./+types/KnowledgeStores";
import { PlusIcon, BookOpenIcon, EllipsisHorizontalIcon, CircleStackIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/StatusBadge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useKnowledgeStores } from "@/api/queries/knowledge";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgeDetailPath, newKnowledgePath } from "@/lib/routes";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeStore, KnowledgeProvider } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Stores | Astro" }];

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);

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
  const { isAuthenticated } = useAuth();
  const { activeAccount: userAccount } = useActiveAccount();

  const navigate = useNavigate();
  const { data, isLoading } = useKnowledgeStores(userAccount, isAuthenticated);
  const stores = data ?? [];

  const [deleteTarget, setDeleteTarget] = useState<KnowledgeStore | null>(null);

  const tableHeaders = ["Name", "Status", "Provider", "Mode", "Storage", "Created"];

  return (
    <div className="flex-1 bg-muted">
      <div className="px-6 py-6">
        <div className="mb-6 flex flex-col-reverse gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
          <div>
            <h1 className="text-heading-1 text-foreground">Knowledge Stores</h1>
            <p className="mt-1 text-[13px] text-muted-foreground">
              Account-level databases shared across agent deployments.
            </p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <Button variant="outline" size="sm">
              <BookOpenIcon className="size-4" />
              Learn more
            </Button>
            <Button size="sm" onClick={() => navigate(newKnowledgePath)}>
              <PlusIcon className="size-4" />
              Add store
            </Button>
          </div>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              {tableHeaders.map((header) => (
                <TableHead key={header}>{header}</TableHead>
              ))}
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <>
                {[1, 2, 3].map((i) => (
                  <TableRow key={i}>
                    <TableCell colSpan={tableHeaders.length + 1}>
                      <Skeleton className="h-5 w-full rounded" />
                    </TableCell>
                  </TableRow>
                ))}
              </>
            ) : stores.length === 0 ? (
              <TableRow>
                <TableCell colSpan={tableHeaders.length + 1} className="p-0">
                  <div className="flex min-h-[260px] flex-col items-center justify-center text-center">
                    <div className="mb-3.5 flex size-10 items-center justify-center rounded-md bg-muted">
                      <CircleStackIcon className="size-5 text-faint-foreground" />
                    </div>
                    <p className="text-heading-4 text-foreground mb-1.5">No knowledge stores yet</p>
                    <p className="text-body text-faint-foreground mb-6">
                      Create a store to give your agents a database for memory, vector search, or caching.
                    </p>
                    <Button onClick={() => navigate(newKnowledgePath)}>
                      <PlusIcon className="size-4" />
                      Add your first store
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              stores.map((store) => (
                <TableRow
                  key={store.id}
                  interactive
                  onClick={() => navigate(knowledgeDetailPath(store.name))}
                >
                  <TableCell className="py-5">
                    <span className="font-medium text-foreground">
                      {store.name}
                    </span>
                  </TableCell>
                  <TableCell className="py-5">
                    <StatusBadge
                      color={statusToColor(store.status)}
                      indicator
                      spinning={isTransitionalStatus(store.status)}
                    >
                      {statusLabel(store.status)}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="py-5 text-muted-foreground">
                    <span className="inline-flex items-center gap-2">
                      {PROVIDERS_WITH_ICON.has(store.provider) ? (
                        <img
                          src={getIntegrationIconUrl(store.provider, "light")}
                          alt=""
                          className="size-5 object-contain"
                          loading="lazy"
                        />
                      ) : (
                        <CircleStackIcon className="size-5 text-muted-foreground/60" />
                      )}
                      {PROVIDER_LABELS[store.provider] ?? store.provider}
                    </span>
                  </TableCell>
                  <TableCell className="py-5">
                    <span className="inline-flex items-center rounded-full border border-border px-2 py-0.5 font-mono text-mono-sm text-muted-foreground">
                      {store.mode === "managed" ? "Managed" : "External"}
                    </span>
                  </TableCell>
                  <TableCell className="py-5 text-muted-foreground">
                    {store.mode === "managed" ? (store.storage ?? "—") : "—"}
                  </TableCell>
                  <TableCell className="py-5 text-muted-foreground">
                    {formatRelativeTime(store.created_at)}
                  </TableCell>
                  <TableCell className="py-5" onClick={(e) => e.stopPropagation()}>
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
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

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
  return <KnowledgeStoresContent />;
}
