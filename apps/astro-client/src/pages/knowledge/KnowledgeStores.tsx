import { useState } from "react";
import { useNavigate } from "react-router";
import type { Route } from "./+types/KnowledgeStores";
import { PlusIcon, EllipsisVerticalIcon, CircleStackIcon, TrashIcon } from "@heroicons/react/24/outline";
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
import { Tag } from "@/components/Tag";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
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
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Knowledge Stores"
        description="Account-level databases shared across agent deployments."
        action={
          isAuthenticated && (
            <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto">
              <PageScopeSwitcher />
              <Button variant="outline" size="sm" asChild>
                <a href="https://docs.astropods.com/private-database" target="_blank" rel="noopener noreferrer">
                  Learn more
                </a>
              </Button>
              <Button size="sm" onClick={() => navigate(newKnowledgePath)}>
                <PlusIcon className="size-4" />
                Add store
              </Button>
            </div>
          )
        }
      />

        {isLoading ? (
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
              {[1, 2, 3].map((i) => (
                <TableRow key={i}>
                  <TableCell colSpan={tableHeaders.length + 1}>
                    <Skeleton className="h-5 w-full rounded" />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : stores.length === 0 ? (
          <div className="rounded-lg border border-dashed border-border-strong px-6 py-12 text-center">
            <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-md bg-border">
              <CircleStackIcon className="size-6 text-muted-foreground" />
            </div>
            <p className="text-heading-3 text-foreground mb-2">No knowledge stores yet</p>
            <p className="text-body text-muted-foreground mb-6 max-w-sm mx-auto">
              Create a store to give your agents a database for memory, vector search, or caching.
            </p>
            <Button onClick={() => navigate(newKnowledgePath)}>
              <PlusIcon className="size-4" />
              Add your first store
            </Button>
          </div>
        ) : (
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
              {stores.map((store) => (
                <TableRow
                  key={store.id}
                  interactive
                  onClick={() => navigate(knowledgeDetailPath(store.name))}
                >
                  <TableCell className="">
                    <span className="font-medium text-foreground">
                      {store.name}
                    </span>
                  </TableCell>
                  <TableCell className="">
                    <StatusBadge
                      color={statusToColor(store.status)}
                      spinning={isTransitionalStatus(store.status)}
                    >
                      {statusLabel(store.status)}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <span className="inline-flex items-center gap-2">
                      {PROVIDERS_WITH_ICON.has(store.provider) ? (
                        <img
                          src={getIntegrationIconUrl(store.provider, "light")}
                          alt=""
                          className="size-4 object-contain"
                          loading="lazy"
                        />
                      ) : (
                        <CircleStackIcon className="size-4 text-muted-foreground/60" />
                      )}
                      {PROVIDER_LABELS[store.provider] ?? store.provider}
                    </span>
                  </TableCell>
                  <TableCell className="">
                    <Tag color={store.mode === "managed" ? "blue" : "default"}>
                      {store.mode === "managed" ? "Managed" : "External"}
                    </Tag>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {store.mode === "managed" ? (store.storage ?? "—") : "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatRelativeTime(store.created_at)}
                  </TableCell>
                  <TableCell className="" onClick={(e) => e.stopPropagation()}>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="size-7">
                          <EllipsisVerticalIcon className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setDeleteTarget(store)}
                        >
                          <TrashIcon className="size-4" />
                          Delete store
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

      {deleteTarget && (
        <DeleteKnowledgeStoreDialog
          open={!!deleteTarget}
          onOpenChange={(o) => { if (!o) setDeleteTarget(null); }}
          storeName={deleteTarget.name}
          account={userAccount}
          boundAgents={deleteTarget.bound_agents}
          onDeleted={() => setDeleteTarget(null)}
        />
      )}
    </PageContainer>
  );
}

export default function KnowledgeStores() {
  return <KnowledgeStoresContent />;
}
