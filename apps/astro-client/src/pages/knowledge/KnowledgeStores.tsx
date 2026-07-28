import { useMemo, useState } from "react";
import { useNavigate } from "react-router";
import type { Route } from "./+types/KnowledgeStores";
import { PlusIcon, EllipsisVerticalIcon, CircleStackIcon, TrashIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { ActionPanel } from "@/components/ui/status-panel";
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
import { AccountFilter } from "@/components/AccountFilter";
import { AccountLoadWarning } from "@/components/AccountLoadWarning";
import { FilteredEmptyState } from "@/components/FilteredEmptyState";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useAuth } from "@/lib/auth";
import { useAccountFilterParam } from "@/hooks/use-account-filter-param";
import {
  useAllAccountsKnowledgeStores,
  type KnowledgeStoreWithAccount,
} from "@/api/queries/all-accounts";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import {
  statusToColor,
  isTransitionalStatus,
  statusLabel,
  PROVIDER_LABELS,
} from "@/components/knowledge/knowledge-utils";
import { knowledgeDetailPath, newKnowledgePath } from "@/lib/routes";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { ResultSetReveal } from "@/components/ui/content-reveal";

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

export default function KnowledgeStores() {
  const { isAuthenticated, accounts } = useAuth();

  const navigate = useNavigate();
  const [accountFilters, setAccountFilters] =
    useAccountFilterParam("knowledge");
  const {
    stores,
    isLoading,
    isError,
    failedAccounts,
    retryFailed,
  } = useAllAccountsKnowledgeStores(isAuthenticated, accountFilters);

  const accountLabels = useMemo(
    () => new Map(accounts.map((a) => [a.name, a.display_name || a.name])),
    [accounts],
  );

  const [deleteTarget, setDeleteTarget] = useState<KnowledgeStoreWithAccount | null>(null);

  const tableHeaders = ["Name", "Account", "Status", "Provider", "Mode", "Storage", "Created"];
  const showEmptyState =
    !isLoading && accountFilters.length === 0 && stores.length === 0 && !isError;
  const showFilteredEmpty =
    !isLoading && accountFilters.length > 0 && stores.length === 0 && !isError;
  const showTotalLoadError =
    isError && failedAccounts.length === 0 && stores.length === 0;

  return (
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Knowledge Stores"
        description="Account-level databases shared across agent deployments."
        action={
          isAuthenticated && (
            <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto">
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

      {isAuthenticated && (stores.length > 0 || accountFilters.length > 0) && (
        <div className="mb-4 flex justify-end">
          <AccountFilter value={accountFilters} onChange={setAccountFilters} />
        </div>
      )}

      {showTotalLoadError ? (
        <div role="alert" className="mb-4">
          <ActionPanel
            tone="error"
            title="Couldn't load knowledge stores"
            primaryLabel="Retry"
            onPrimary={retryFailed}
          >
            The knowledge store list is temporarily unavailable.
          </ActionPanel>
        </div>
      ) : isError ? (
        <div className="mb-4">
          <AccountLoadWarning failedAccounts={failedAccounts} onRetry={retryFailed} />
        </div>
      ) : null}

      <ResultSetReveal
        itemCount={stores.length}
        settled={!isLoading}
        transitionKey={accountFilters.join(",")}
      >
        {showTotalLoadError ? null : showEmptyState ? (
          <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
            <div className="flex justify-center mb-3 text-muted-foreground">
              <CircleStackIcon className="size-6" />
            </div>
            <p className="text-sm font-medium text-foreground">No knowledge stores yet</p>
            <p className="text-xs text-muted-foreground mt-1 mb-4">
              Create a store to give your agents a database for memory, vector search, or caching.
            </p>
            <Button onClick={() => navigate(newKnowledgePath)}>
              <PlusIcon className="size-4" />
              Add your first store
            </Button>
          </div>
        ) : showFilteredEmpty ? (
          <FilteredEmptyState
            message="No knowledge stores match your filters."
            onClear={() => setAccountFilters([])}
          />
        ) : stores.length > 0 ? (
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
                  key={`${store.account}/${store.id}`}
                  interactive
                  onClick={() => navigate(knowledgeDetailPath(store.name, store.account))}
                >
                  <TableCell>
                    <span className="font-medium text-foreground">
                      {store.name}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {accountLabels.get(store.account) ?? store.account}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      color={statusToColor(store.status)}
                      spinning={isTransitionalStatus(store.status)}
                    >
                      {statusLabel(store.status)}
                    </StatusBadge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <span className="inline-flex items-center gap-2">
                      <ProviderIcon provider={store.provider} className="size-4" />
                      {PROVIDER_LABELS[store.provider] ?? store.provider}
                    </span>
                  </TableCell>
                  <TableCell>
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
                  <TableCell onClick={(e) => e.stopPropagation()}>
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
        ) : null}
      </ResultSetReveal>

      {deleteTarget && (
        <DeleteKnowledgeStoreDialog
          open={!!deleteTarget}
          onOpenChange={(o) => { if (!o) setDeleteTarget(null); }}
          storeName={deleteTarget.name}
          account={deleteTarget.account}
          boundAgents={deleteTarget.bound_agents}
          onDeleted={() => setDeleteTarget(null)}
        />
      )}
    </PageContainer>
  );
}
