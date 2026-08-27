import { useCallback, useMemo, useState } from "react";
import { Link } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { USER_BLUEPRINTS_PAGE_SIZE, useUserBlueprints } from "@/api/queries/blueprints";
import { blueprintKeys } from "@/api/queries/keys";
import { AccountScopeFilter } from "@/components/AccountScopeFilter";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { BlueprintsEmptyState } from "@/components/blueprint/BlueprintsEmptyState";
import { FilteredEmptyState } from "@/components/FilteredEmptyState";
import { DebouncedFilterInput } from "@/components/DebouncedFilterInput";
import { IndeterminateProgressBar } from "@/components/IndeterminateProgressBar";
import { ListPagination } from "@/components/ListPagination";
import { ListResultsTransition } from "@/components/ListResultsTransition";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { Button } from "@/components/ui/button";
import { useOrgScope } from "@/hooks/use-org-scope";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { firstInfinitePage, usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { loadOrgScoped } from "@/lib/api.server";
import { useAuth } from "@/lib/auth";
import type { BlueprintListParams } from "@/lib/blueprint-list-params";
import { shouldRevalidateUserResourceList } from "@/lib/user-resource-revalidation";
import { useBlueprintSearch } from "./use-blueprint-search";
import type { Route } from "./+types/Blueprints";

export const meta: Route.MetaFunction = () => [{ title: "Blueprints | Astro" }];
export const shouldRevalidate = shouldRevalidateUserResourceList;

const FIRST_PAGE_PARAMS: BlueprintListParams = {
  limit: USER_BLUEPRINTS_PAGE_SIZE,
};

export async function loader({ request }: Route.LoaderArgs) {
  const scoped = await loadOrgScoped(request, (api, scope) =>
    api.listUserBlueprints(scope, FIRST_PAGE_PARAMS),
  );
  return { ...scoped, firstPageParams: FIRST_PAGE_PARAMS };
}

export default function Blueprints({ loaderData }: Route.ComponentProps) {
  const { accounts, isAuthenticated } = useAuth();
  const { account, setAccount, scope } = useOrgScope();
  const { search, setSearch, params, hasActiveFilters } = useBlueprintSearch();

  usePrimeQueryCache(loaderData, (queryClient, data) => {
    if (!data?.scope || !data.data) return;
    queryClient.setQueryData(
      blueprintKeys.visibleList(data.scope, data.firstPageParams),
      firstInfinitePage(data.data),
    );
  });

  const query = useUserBlueprints(scope, params, isAuthenticated);
  const blueprintPages = query.data?.pages ?? [];
  const pagination = useCursorPagination({
    pages: blueprintPages,
    hasNextPage: !!query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: query.fetchNextPage,
    resetKey: JSON.stringify([scope.all, scope.accounts, params]),
  });
  const blueprints = pagination.page?.blueprints ?? [];
  const ownerAccounts = useMemo(() => new Set(accounts.map((account) => account.name)), [accounts]);
  const loadingFirstPage = query.isPending && blueprints.length === 0;
  const settled = isAuthenticated && !query.isPending && !query.isFetching;
  const showToolbar = loadingFirstPage || blueprints.length > 0 || hasActiveFilters;
  const showFilteredEmpty = settled && blueprints.length === 0 && hasActiveFilters;
  const showRegistryEmpty = settled && blueprints.length === 0 && !hasActiveFilters && !query.isError;
  const listError = query.isError && blueprints.length === 0;

  const [filterResetKey, setFilterResetKey] = useState(0);
  const clearFilters = useCallback(() => {
    setSearch("");
    setFilterResetKey((key) => key + 1);
  }, [setSearch]);

  return (
    <PageContainer outerClassName="bg-background">
      <IndeterminateProgressBar active={query.isFetching && blueprints.length > 0} />
      <PageHeader
        title="Blueprints for"
        adornment={<AccountScopeFilter value={account} onChange={setAccount} className="-ml-1" />}
        description="Agent configurations available to deploy in this account."
        action={isAuthenticated ? (
          <Button asChild size="sm">
            <Link to="/new/custom">
              <PlusIcon className="size-4" />
              Create blueprint
            </Link>
          </Button>
        ) : undefined}
      />

      {showToolbar && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <DebouncedFilterInput
            placeholder="Search blueprints…"
            value={search}
            resetKey={filterResetKey}
            onDebouncedChange={setSearch}
            containerClassName="h-8 w-full min-w-[12rem] max-w-sm flex-1 bg-card dark:bg-background sm:max-w-xs"
          />
        </div>
      )}

      <ListResultsTransition
        transitionKey={JSON.stringify([
          scope.all,
          scope.accounts,
          params,
          pagination.currentPage,
          loadingFirstPage,
        ])}
      >
        {showFilteredEmpty ? (
          <FilteredEmptyState message="No blueprints match your filters." onClear={clearFilters} />
        ) : (
          <BlueprintListView
            blueprints={blueprints}
            isLoading={loadingFirstPage}
            isError={listError}
            error={query.error}
            refetch={() => void query.refetch()}
            emptyContent={showRegistryEmpty ? <BlueprintsEmptyState /> : null}
            ownerAccounts={ownerAccounts}
            slotCount={pagination.totalPages > 1 ? USER_BLUEPRINTS_PAGE_SIZE : undefined}
          />
        )}
      </ListResultsTransition>
      {blueprints.length > 0 && (
        <ListPagination
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          onPageChange={pagination.onPageChange}
          disabled={query.isFetchingNextPage}
          ariaLabel="Blueprint list pagination"
        />
      )}
    </PageContainer>
  );
}
