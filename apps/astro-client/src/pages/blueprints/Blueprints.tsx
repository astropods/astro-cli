import { useCallback, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { blueprintKeys } from "@/api/queries/keys";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { BlueprintsEmptyState } from "@/components/blueprint/BlueprintsEmptyState";
import { FilterInput } from "@/components/FilterInput";
import { IndeterminateProgressBar } from "@/components/IndeterminateProgressBar";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import {
  BLUEPRINT_LIST_DEFAULT_PAGE_SIZE,
  hasBlueprintListFilters,
  type BlueprintListParams,
} from "@/lib/blueprint-list-params";
import { useActiveAccount } from "@/hooks/use-active-account";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { useAuth } from "@/lib/auth";
import type { Blueprint } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { loadAccountScoped } from "@/lib/api.server";
import { blueprintGridSlotCount } from "@/lib/blueprint-page-numbers";
import { useAccountBlueprintsList } from "./use-account-blueprints-list";
import { useBlueprintSearch } from "./use-blueprint-search";
import { BlueprintsPagination } from "./BlueprintsPagination";
import type { Route } from "./+types/Blueprints";

export const meta: Route.MetaFunction = () => [{ title: "Blueprints | Astro" }];

const FIRST_PAGE_PARAMS: BlueprintListParams = {
  limit: BLUEPRINT_LIST_DEFAULT_PAGE_SIZE,
  offset: 0,
};

export async function loader({ request }: Route.LoaderArgs) {
  const scoped = await loadAccountScoped(request, (api, account) =>
    api.listAccountBlueprints(account, FIRST_PAGE_PARAMS),
  );
  return { ...scoped, firstPageParams: FIRST_PAGE_PARAMS };
}

export default function Blueprints({ loaderData }: Route.ComponentProps) {
  const { activeAccount, setActiveAccount } = useActiveAccount();
  const { accounts, isAuthenticated } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const { search, setSearch, params, hasActiveFilters } = useBlueprintSearch();
  const [page, setPage] = useState(1);

  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (ld?.account && ld?.data && ld?.firstPageParams) {
      qc.setQueryData(blueprintKeys.list(ld.account, ld.firstPageParams), ld.data);
    }
  });

  // Consume ?account= param once accounts have loaded so deep-links and
  // org-switcher redirects land on the right scope without a flash.
  useEffect(() => {
    const accountParam = searchParams.get("account");
    if (!accountParam || accounts.length === 0) return;
    if (accounts.some((a) => a.name === accountParam)) {
      setActiveAccount(accountParam);
    }
    setSearchParams({}, { replace: true });
  }, [accounts, searchParams, setActiveAccount, setSearchParams]);

  useEffect(() => {
    setPage(1);
  }, [activeAccount, params]);

  const handlePageChange = useCallback((nextPage: number) => {
    setPage(nextPage);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);

  const isReady = isAuthenticated && !!activeAccount;
  const {
    data,
    isPending,
    isFetching,
    isError,
    error,
    refetch,
  } = useAccountBlueprintsList(activeAccount, {
    enabled: isReady,
    params: { ...params, limit: BLUEPRINT_LIST_DEFAULT_PAGE_SIZE },
    page,
  });

  const blueprints = data?.agents ?? [];
  const totalCount = data?.count ?? 0;
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  const waitingForAccount = !isReady;
  const loadingFirstPage = isPending;
  const loadingAfterFilterChange = hasActiveFilters && isFetching;

  const listLoading = waitingForAccount || loadingFirstPage || loadingAfterFilterChange;
  const listRefetching = isReady && isFetching && blueprints.length > 0;
  const fetchSettled = isReady && !isPending && !isFetching;
  const isEmpty = blueprints.length === 0;
  const hasTypedSearch = search.trim().length > 0;

  const showToolbar =
    listLoading ||
    isFetching ||
    !isEmpty ||
    hasActiveFilters ||
    hasTypedSearch;

  const showFilteredEmpty = fetchSettled && isEmpty && hasActiveFilters;
  const showRegistryEmpty = fetchSettled && isEmpty && !hasBlueprintListFilters(params);

  const gridSlotCount = blueprintGridSlotCount({
    showFilteredEmpty,
    totalCount,
    pageSize: BLUEPRINT_LIST_DEFAULT_PAGE_SIZE,
  });

  return (
    <PageContainer outerClassName="bg-background">
      <IndeterminateProgressBar active={listRefetching} />
      <PageHeader
        title="Blueprints"
        description="Agent configurations available to deploy in your account."
        action={<BlueprintsHeaderActions visible={isAuthenticated} />}
      />

      <BlueprintsToolbar
        visible={showToolbar}
        search={search}
        onSearchChange={setSearch}
      />

      <BlueprintsListArea
        showFilteredEmpty={showFilteredEmpty}
        showRegistryEmpty={showRegistryEmpty}
        blueprints={blueprints}
        totalCount={totalCount}
        page={page}
        pageSize={BLUEPRINT_LIST_DEFAULT_PAGE_SIZE}
        gridSlotCount={gridSlotCount}
        listLoading={listLoading}
        paginationDisabled={isFetching}
        isError={isError}
        error={error}
        refetch={refetch}
        ownerAccounts={ownerAccounts}
        onPageChange={handlePageChange}
      />
    </PageContainer>
  );
}

function BlueprintsHeaderActions({ visible }: { visible: boolean }) {
  if (!visible) {
    return null;
  }

  return (
    <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto">
      <PageScopeSwitcher />
      <Button asChild size="sm">
        <Link to="/new/custom">
          <PlusIcon className="size-4" />
          Create blueprint
        </Link>
      </Button>
    </div>
  );
}

function BlueprintsToolbar({
  visible,
  search,
  onSearchChange,
}: {
  visible: boolean;
  search: string;
  onSearchChange: (value: string) => void;
}) {
  if (!visible) {
    return null;
  }

  return (
    <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
      <FilterInput
        placeholder="Search blueprints…"
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        containerClassName="h-8 w-full min-w-[12rem] max-w-sm flex-1 bg-card dark:bg-background sm:max-w-xs"
      />
    </div>
  );
}

function BlueprintsListArea({
  showFilteredEmpty,
  showRegistryEmpty,
  blueprints,
  totalCount,
  page,
  pageSize,
  gridSlotCount,
  listLoading,
  paginationDisabled,
  isError,
  error,
  refetch,
  ownerAccounts,
  onPageChange,
}: {
  showFilteredEmpty: boolean;
  showRegistryEmpty: boolean;
  blueprints: Blueprint[];
  totalCount: number;
  page: number;
  pageSize: number;
  gridSlotCount?: number;
  listLoading: boolean;
  paginationDisabled: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  ownerAccounts: Set<string>;
  onPageChange: (page: number) => void;
}) {
  if (showFilteredEmpty) {
    return <BlueprintsFilteredEmptyMessage />;
  }

  return (
    <>
      <BlueprintListView
        blueprints={blueprints}
        isLoading={listLoading}
        isError={isError}
        error={error}
        refetch={refetch}
        emptyContent={<BlueprintsRegistryEmpty visible={showRegistryEmpty} />}
        ownerAccounts={ownerAccounts}
        slotCount={gridSlotCount}
        showAuthor
      />
      <BlueprintsPagination
        currentPage={page}
        totalCount={totalCount}
        pageSize={pageSize}
        onPageChange={onPageChange}
        disabled={paginationDisabled}
      />
    </>
  );
}

function BlueprintsRegistryEmpty({ visible }: { visible: boolean }) {
  if (!visible) {
    return null;
  }
  return <BlueprintsEmptyState />;
}

function BlueprintsFilteredEmptyMessage() {
  return (
    <div className="rounded-lg border border-border p-8 text-center">
      <p className="text-body-sm text-muted-foreground">No blueprints match your filters.</p>
    </div>
  );
}
