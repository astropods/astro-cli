import { useCallback, useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { BlueprintsEmptyState } from "@/components/blueprint/BlueprintsEmptyState";
import { AccountFilter } from "@/components/AccountFilter";
import { AccountLoadWarning } from "@/components/AccountLoadWarning";
import { FilteredEmptyState } from "@/components/FilteredEmptyState";
import { FilterInput } from "@/components/FilterInput";
import { IndeterminateProgressBar } from "@/components/IndeterminateProgressBar";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { BLUEPRINT_LIST_DEFAULT_PAGE_SIZE } from "@/lib/blueprint-list-params";
import { useAccountFilterParam } from "@/hooks/use-account-filter-param";
import { useAuth } from "@/lib/auth";
import {
  getBlueprintCategories,
  getBlueprintDescription,
  getLatestVersion,
} from "@/lib/blueprint-utils";
import type { Blueprint } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { blueprintGridSlotCount } from "@/lib/blueprint-page-numbers";
import { useAllAccountsBlueprints } from "@/api/queries/all-accounts";
import { useBlueprintSearch } from "./use-blueprint-search";
import { BlueprintsPagination } from "./BlueprintsPagination";
import type { Route } from "./+types/Blueprints";

export const meta: Route.MetaFunction = () => [{ title: "Blueprints | Astro" }];

export default function Blueprints() {
  const { accounts, isAuthenticated } = useAuth();
  const { search, setSearch, params, hasActiveFilters } = useBlueprintSearch();
  const [page, setPage] = useState(1);
  const [accountFilters, setAccountFilters] = useAccountFilterParam();

  useEffect(() => {
    setPage(1);
  }, [accountFilters, params]);

  const handlePageChange = useCallback((nextPage: number) => {
    setPage(nextPage);
    window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);
  const handleClearFilters = useCallback(() => {
    setSearch("");
    setAccountFilters([]);
  }, [setAccountFilters, setSearch]);

  const {
    blueprints,
    isLoading,
    isFetching,
    isError,
    error,
    refetch,
    failedAccounts,
    retryFailed,
  } = useAllAccountsBlueprints(isAuthenticated, accountFilters);

  const filtered = useMemo(() => {
    let list = blueprints;
    const q = params.q?.trim().toLowerCase();
    if (q) {
      // Match name, account, description, and tags to keep parity with the
      // server-side search this replaced (which matched name/description/tags).
      list = list.filter(
        (b) =>
          b.name.toLowerCase().includes(q) ||
          b.account.toLowerCase().includes(q) ||
          getBlueprintDescription(b).toLowerCase().includes(q) ||
          getBlueprintCategories(b).some((tag) => tag.toLowerCase().includes(q)),
      );
    }
    const publishedAt = (b: Blueprint) => {
      const at = getLatestVersion(b)?.published_at;
      return at ? new Date(at).getTime() : 0;
    };
    return [...list].sort((a, b) => publishedAt(b) - publishedAt(a));
  }, [blueprints, params.q]);

  const totalCount = filtered.length;
  const pageCount = Math.max(
    1,
    Math.ceil(totalCount / BLUEPRINT_LIST_DEFAULT_PAGE_SIZE),
  );
  const effectivePage = Math.min(page, pageCount);
  const pageStart = (effectivePage - 1) * BLUEPRINT_LIST_DEFAULT_PAGE_SIZE;
  const pageBlueprints = filtered.slice(
    pageStart,
    pageStart + BLUEPRINT_LIST_DEFAULT_PAGE_SIZE,
  );
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  const listLoading = !isAuthenticated || isLoading;
  const listRefetching = isAuthenticated && isFetching && blueprints.length > 0;
  const fetchSettled = isAuthenticated && !isLoading && !isFetching;
  const isEmpty = filtered.length === 0;
  const hasTypedSearch = search.trim().length > 0;
  const hasAnyFilter = hasActiveFilters || accountFilters.length > 0;
  const listError = isError && blueprints.length === 0;

  const showToolbar = listLoading || !isEmpty || hasAnyFilter || hasTypedSearch;
  const showFilteredEmpty = fetchSettled && isEmpty && hasAnyFilter;
  const showRegistryEmpty = fetchSettled && isEmpty && !hasAnyFilter && !listError;

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
        description="Agent configurations available to deploy across your accounts."
        action={
          isAuthenticated ? (
            <Button asChild size="sm">
              <Link to="/new/custom">
                <PlusIcon className="size-4" />
                Create blueprint
              </Link>
            </Button>
          ) : undefined
        }
      />

      {showToolbar && (
        <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
          <FilterInput
            placeholder="Search blueprints…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            containerClassName="h-8 w-full min-w-[12rem] max-w-sm flex-1 bg-card dark:bg-background sm:max-w-xs"
          />
          <AccountFilter value={accountFilters} onChange={setAccountFilters} />
        </div>
      )}

      {isError && !listError && (
        <div className="mb-4">
          <AccountLoadWarning failedAccounts={failedAccounts} onRetry={retryFailed} />
        </div>
      )}

      {showFilteredEmpty ? (
        <FilteredEmptyState
          message="No blueprints match your filters."
          onClear={handleClearFilters}
        />
      ) : (
        <>
          <BlueprintListView
            blueprints={pageBlueprints}
            isLoading={listLoading}
            isError={listError}
            error={error}
            refetch={refetch}
            emptyContent={showRegistryEmpty ? <BlueprintsEmptyState /> : null}
            ownerAccounts={ownerAccounts}
            slotCount={gridSlotCount}
          />
          <BlueprintsPagination
            currentPage={effectivePage}
            totalCount={totalCount}
            pageSize={BLUEPRINT_LIST_DEFAULT_PAGE_SIZE}
            onPageChange={handlePageChange}
            disabled={isFetching}
          />
        </>
      )}
    </PageContainer>
  );
}
