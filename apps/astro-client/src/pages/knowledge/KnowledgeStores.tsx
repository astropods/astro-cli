import { useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { CircleStackIcon, EllipsisVerticalIcon, PlusIcon, TrashIcon } from "@heroicons/react/24/outline";
import type { Route } from "./+types/KnowledgeStores";
import { USER_KNOWLEDGE_PAGE_SIZE, useUserKnowledgeStores } from "@/api/queries/knowledge";
import { knowledgeKeys } from "@/api/queries/keys";
import { AccountFilter } from "@/components/AccountFilter";
import { FilteredEmptyState } from "@/components/FilteredEmptyState";
import { DebouncedFilterInput } from "@/components/DebouncedFilterInput";
import { ListPagination } from "@/components/ListPagination";
import { ListResultsTransition } from "@/components/ListResultsTransition";
import { DeleteKnowledgeStoreDialog } from "@/components/knowledge/DeleteKnowledgeStoreDialog";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import {
  displayProvider,
  isTransitionalStatus,
  PROVIDER_LABELS,
  statusLabel,
  statusToColor,
} from "@/components/knowledge/knowledge-utils";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { StatusBadge } from "@/components/StatusBadge";
import { Tag } from "@/components/Tag";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ActionPanel } from "@/components/ui/status-panel";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { usePersistentAccountFilterParam } from "@/hooks/use-account-filter-param";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useUserResourceSearch } from "@/hooks/use-user-resource-search";
import { firstInfinitePage, usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { loadUserResourceScoped } from "@/lib/api.server";
import type { KnowledgeStore } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { knowledgeDetailPath, newKnowledgePath } from "@/lib/routes";
import { resolveUserResourceScope } from "@/lib/user-resource-scope";
import { shouldRevalidateUserResourceList } from "@/lib/user-resource-revalidation";

export const meta: Route.MetaFunction = () => [{ title: "Knowledge Stores | Astro" }];
export const shouldRevalidate = shouldRevalidateUserResourceList;

export async function loader({ request }: Route.LoaderArgs) {
  return loadUserResourceScoped(request, (api, scope) =>
    api.listUserKnowledgeStores(scope, { limit: USER_KNOWLEDGE_PAGE_SIZE }),
  );
}

function formatRelativeTime(dateStr: string): string {
  const diffMin = Math.floor((Date.now() - new Date(dateStr).getTime()) / 60000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHours = Math.floor(diffMin / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${Math.floor(diffHours / 24)}d ago`;
}

export default function KnowledgeStores({ loaderData }: Route.ComponentProps) {
  const { accounts, isAuthenticated } = useAuth();
  const navigate = useNavigate();
  const [
    accountFilters,
    setAccountFilters,
    hasExplicitAccountFilter,
    resetAccountFilters,
  ] = usePersistentAccountFilterParam("knowledge");
  const scope = useMemo(
    () => resolveUserResourceScope(accountFilters, accounts.map((account) => account.name)),
    [accountFilters, accounts],
  );
  const { search, setSearch, params, hasActiveSearch } = useUserResourceSearch();

  usePrimeQueryCache(loaderData, (queryClient, data) => {
    if (!data?.scope || !data.data) return;
    queryClient.setQueryData(
      knowledgeKeys.visibleList(data.scope, { limit: USER_KNOWLEDGE_PAGE_SIZE }),
      firstInfinitePage(data.data),
    );
  });

  const query = useUserKnowledgeStores(scope, params, isAuthenticated);
  const knowledgePages = query.data?.pages ?? [];
  const pagination = useCursorPagination({
    pages: knowledgePages,
    hasNextPage: !!query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: query.fetchNextPage,
    resetKey: JSON.stringify([scope.all, scope.accounts, params]),
  });
  const stores = pagination.page?.stores ?? [];
  const accountLabels = useMemo(
    () => new Map(accounts.map((account) => [account.name, account.display_name || account.name])),
    [accounts],
  );
  const [deleteTarget, setDeleteTarget] = useState<KnowledgeStore | null>(null);
  const hasAnyFilter = hasExplicitAccountFilter || hasActiveSearch;
  const showEmptyState = !query.isPending && !hasAnyFilter && stores.length === 0 && !query.isError;
  const showFilteredEmpty = !query.isPending && hasAnyFilter && stores.length === 0 && !query.isError;
  const showTotalLoadError = query.isError && stores.length === 0;
  const showToolbar = isAuthenticated && (
    stores.length > 0 || hasAnyFilter || query.isPending
  );

  const [filterResetKey, setFilterResetKey] = useState(0);
  const clearFilters = () => {
    setSearch("");
    resetAccountFilters();
    setFilterResetKey((key) => key + 1);
  };

  return (
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Knowledge Stores"
        description="Account-level databases shared across agent deployments."
        action={isAuthenticated && (
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
        )}
      />

      {showToolbar && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <DebouncedFilterInput
            placeholder="Search knowledge stores…"
            value={search}
            resetKey={filterResetKey}
            onDebouncedChange={setSearch}
            containerClassName="h-8 w-full min-w-[12rem] max-w-sm flex-1 bg-card dark:bg-background sm:max-w-xs"
          />
          <AccountFilter value={accountFilters} onChange={setAccountFilters} />
        </div>
      )}
      {showEmptyState && (
        <div className="mb-4 flex justify-end">
          <AccountFilter
            className="w-full @[480px]:w-auto @[480px]:min-w-[13rem]"
            value={accountFilters}
            onChange={setAccountFilters}
          />
        </div>
      )}

      <ListResultsTransition
        transitionKey={JSON.stringify([
          scope.all,
          scope.accounts,
          params,
          pagination.currentPage,
          query.isPending,
        ])}
      >
        {showTotalLoadError ? (
          <div role="alert" className="mb-4">
            <ActionPanel
              tone="error"
              title="Couldn't load knowledge stores"
              primaryLabel="Retry"
              onPrimary={() => void query.refetch()}
            >
              The knowledge store list is temporarily unavailable.
            </ActionPanel>
          </div>
        ) : showEmptyState ? (
          <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
            <div className="mb-3 flex justify-center text-muted-foreground">
              <CircleStackIcon className="size-6" />
            </div>
            <p className="text-sm font-medium text-foreground">No knowledge stores yet</p>
            <p className="mt-1 mb-4 text-xs text-muted-foreground">
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
            onClear={clearFilters}
          />
        ) : stores.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                {["Name", "Account", "Status", "Provider", "Mode", "Storage", "Created"].map((header) => (
                  <TableHead key={header}>{header}</TableHead>
                ))}
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {stores.map((store) => {
                const owner = store.account;
                return (
                  <TableRow
                    key={`${owner ?? "missing-account"}/${store.id}`}
                    interactive={!!owner}
                    onClick={owner ? () => navigate(knowledgeDetailPath(store.name, owner)) : undefined}
                  >
                    <TableCell><span className="font-medium text-foreground">{store.name}</span></TableCell>
                    <TableCell className="text-muted-foreground">
                      {owner ? (accountLabels.get(owner) ?? owner) : "Unavailable"}
                    </TableCell>
                    <TableCell>
                      <StatusBadge color={statusToColor(store.status)} spinning={isTransitionalStatus(store.status)}>
                        {statusLabel(store.status)}
                      </StatusBadge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      <span className="inline-flex items-center gap-2">
                        <ProviderIcon provider={displayProvider(store)} className="size-4" />
                        {PROVIDER_LABELS[displayProvider(store)] ?? store.provider}
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
                    <TableCell className="text-muted-foreground">{formatRelativeTime(store.created_at)}</TableCell>
                    <TableCell onClick={(event) => event.stopPropagation()}>
                      {owner && (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" className="size-7">
                              <EllipsisVerticalIcon className="size-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem variant="destructive" onClick={() => setDeleteTarget(store)}>
                              <TrashIcon className="size-4" />
                              Delete store
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        ) : null}
      </ListResultsTransition>
      {stores.length > 0 && (
        <ListPagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          onPageChange={pagination.onPageChange}
          disabled={query.isFetchingNextPage}
          ariaLabel="Knowledge store list pagination"
        />
      )}

      {deleteTarget?.account && (
        <DeleteKnowledgeStoreDialog
          open
          onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}
          storeName={deleteTarget.name}
          account={deleteTarget.account}
          boundAgents={deleteTarget.bound_agents}
          onDeleted={() => setDeleteTarget(null)}
        />
      )}
    </PageContainer>
  );
}
