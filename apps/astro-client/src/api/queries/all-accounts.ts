import { useCallback, useEffect, useMemo } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-context";
import { useAuth } from "@/lib/auth";
import type {
  AgentDeploymentSummary,
  BlueprintsListResponse,
  CrossAccountResourceResponse,
  DeploymentsListResponse,
  KnowledgeStore,
  KnowledgeStoreListResponse,
} from "@/lib/api";
import { allAccountKeys, type AllAccountResource } from "./keys";

const TRANSITIONAL_KNOWLEDGE = ["provisioning", "connecting", "pending-acceptance"];
const TRANSITIONAL_DEPLOYMENT = ["pending", "provisioning", "deploying", "undeploying"];
const AGGREGATE_PAGE_SIZE = 100;
const AGGREGATE_MAX_OFFSET = 10_000;

export type KnowledgeStoreWithAccount = KnowledgeStore & { account: string };
export type DeploymentWithAccount = AgentDeploymentSummary & { account: string };

async function loadCompleteAccountPages<T>(
  accounts: string[],
  loadPage: (
    accounts: string[],
    page: { limit: number; offset: number },
  ) => Promise<CrossAccountResourceResponse<T>>,
  mergeData: (current: T | undefined, page: T) => T,
  dataHasMore: (data: T) => boolean,
  pageMadeProgress?: (current: T | undefined, page: T) => boolean,
): Promise<CrossAccountResourceResponse<T>> {
  const results = new Map<string, CrossAccountResourceResponse<T>["results"][number]>();
  const failed = new Set<string>();
  const rejected = new Set<string>();
  let pending = accounts;
  let offset = 0;

  while (pending.length > 0) {
    const response = await loadPage(pending, { limit: AGGREGATE_PAGE_SIZE, offset });
    const returned = new Set(response.results.map((result) => result.account));

    // A later-page failure invalidates that account's earlier pages. Consumers
    // search and sort these catalogs as complete, so partial data would mislead.
    for (const account of response.failed_accounts) {
      failed.add(account);
      results.delete(account);
    }
    for (const account of response.rejected_accounts ?? []) {
      rejected.add(account);
      results.delete(account);
    }
    for (const account of pending) {
      if (!returned.has(account) && !failed.has(account) && !rejected.has(account)) {
        failed.add(account);
        results.delete(account);
      }
    }

    const next: string[] = [];
    for (const result of response.results) {
      if (failed.has(result.account) || rejected.has(result.account)) continue;
      const current = results.get(result.account);
      const madeProgress = pageMadeProgress?.(current?.data, result.data) ?? true;
      results.set(result.account, {
        ...result,
        data: mergeData(current?.data, result.data),
        offset: 0,
        has_more: false,
      });
      if (result.has_more ?? dataHasMore(result.data)) {
        if (madeProgress && offset + AGGREGATE_PAGE_SIZE <= AGGREGATE_MAX_OFFSET) {
          next.push(result.account);
        }
      }
    }
    pending = next;
    offset += AGGREGATE_PAGE_SIZE;
  }

  return {
    results: accounts.flatMap((account) => {
      const result = results.get(account);
      return result ? [result] : [];
    }),
    failed_accounts: accounts.filter((account) => failed.has(account)),
    rejected_accounts: accounts.filter((account) => rejected.has(account)),
  };
}

function mergeResponses<T>(
  current: CrossAccountResourceResponse<T> | undefined,
  update: CrossAccountResourceResponse<T>,
): CrossAccountResourceResponse<T> {
  if (!current) return update;
  const updated = new Set([
    ...update.results.map((result) => result.account),
    ...update.failed_accounts,
    ...(update.rejected_accounts ?? []),
  ]);
  return {
    results: [
      ...current.results.filter((result) => !updated.has(result.account)),
      ...update.results,
    ],
    failed_accounts: [
      ...current.failed_accounts.filter((account) => !updated.has(account)),
      ...update.failed_accounts,
    ],
    rejected_accounts: [
      ...(current.rejected_accounts ?? []).filter((account) => !updated.has(account)),
      ...(update.rejected_accounts ?? []),
    ],
  };
}

function selectResponseAccounts<T>(
  response: CrossAccountResourceResponse<T>,
  accounts: string[],
): CrossAccountResourceResponse<T> {
  const results = new Map(response.results.map((result) => [result.account, result]));
  const failed = new Set(response.failed_accounts);
  const rejected = new Set(response.rejected_accounts ?? []);
  return {
    results: accounts.flatMap((account) => {
      const result = results.get(account);
      return result ? [result] : [];
    }),
    failed_accounts: accounts.filter((account) => failed.has(account)),
    rejected_accounts: accounts.filter((account) => rejected.has(account)),
  };
}

function findCachedSuperset<T>(
  queryClient: QueryClient,
  resource: AllAccountResource,
  accounts: string[],
): { data: CrossAccountResourceResponse<T>; dataUpdatedAt: number } | undefined {
  let match:
    | { data: CrossAccountResourceResponse<T>; dataUpdatedAt: number }
    | undefined;

  for (const [queryKey, data] of queryClient.getQueriesData<
    CrossAccountResourceResponse<T>
  >({ queryKey: allAccountKeys.resource(resource) })) {
    if (!data || queryKey[2] !== "list") continue;
    const cachedAccounts = queryKey[3];
    if (
      !Array.isArray(cachedAccounts) ||
      !accounts.every((account) => cachedAccounts.includes(account))
    ) {
      continue;
    }

    const state = queryClient.getQueryState(queryKey);
    if (!state?.dataUpdatedAt || state.isInvalidated) continue;
    if (!match || state.dataUpdatedAt > match.dataUpdatedAt) {
      match = {
        data: selectResponseAccounts(data, accounts),
        dataUpdatedAt: state.dataUpdatedAt,
      };
    }
  }

  return match;
}

function useAllAccountResource<T>({
  resource,
  enabled,
  accountFilters,
  load,
  isTransitional,
}: {
  resource: AllAccountResource;
  enabled: boolean;
  accountFilters?: string[];
  load: (accounts: string[]) => Promise<CrossAccountResourceResponse<T>>;
  isTransitional?: (data: T) => boolean;
}) {
  const { accounts } = useAuth();
  const accountNames = useMemo(() => {
    const membershipNames = accounts.map((account) => account.name);
    if (!accountFilters?.length) return membershipNames;
    const selected = new Set(accountFilters);
    return membershipNames.filter((name) => selected.has(name));
  }, [accountFilters, accounts]);
  const queryClient = useQueryClient();
  const queryKey = useMemo(
    () => allAccountKeys.list(resource, accountNames),
    [accountNames, resource],
  );
  const cachedSuperset = useMemo(
    () => findCachedSuperset<T>(queryClient, resource, accountNames),
    [accountNames, queryClient, resource],
  );
  const query = useQuery({
    queryKey,
    queryFn: () => load(accountNames),
    enabled: enabled && accountNames.length > 0,
    initialData: cachedSuperset?.data,
    initialDataUpdatedAt: cachedSuperset?.dataUpdatedAt,
  });

  const merge = useCallback(
    (update: CrossAccountResourceResponse<T>) => {
      queryClient.setQueryData<CrossAccountResourceResponse<T>>(queryKey, (current) =>
        mergeResponses(current, update),
      );
    },
    [queryClient, queryKey],
  );

  const pollingAccounts = useMemo(
    () =>
      isTransitional
        ? (query.data?.results ?? [])
            .filter((result) => isTransitional(result.data))
            .map((result) => result.account)
        : [],
    [isTransitional, query.data],
  );

  const pollQuery = useQuery({
    queryKey: allAccountKeys.target(resource, "poll", pollingAccounts),
    queryFn: () => load(pollingAccounts),
    enabled: enabled && pollingAccounts.length > 0,
    refetchInterval: 3000,
    staleTime: 0,
  });

  useEffect(() => {
    if (pollQuery.data) merge(pollQuery.data);
  }, [merge, pollQuery.data]);

  const failedAccounts = query.data?.failed_accounts;
  const retryFailed = useCallback(() => {
    if (!failedAccounts?.length) {
      void query.refetch();
      return;
    }
    void queryClient
      .fetchQuery({
        queryKey: allAccountKeys.target(resource, "retry", failedAccounts),
        queryFn: () => load(failedAccounts),
        staleTime: 0,
      })
      .then(merge)
      .catch(() => undefined);
  }, [failedAccounts, load, merge, query, queryClient, resource]);

  return {
    ...query,
    isError: query.isError || !!failedAccounts?.length,
    failedAccounts: failedAccounts ?? [],
    retryFailed,
  };
}

export function useAllAccountsBlueprints(
  enabled = true,
  accountFilters?: string[],
) {
  const api = useApiClient();
  const load = useCallback(
    (accounts: string[]) =>
      loadCompleteAccountPages(
        accounts,
        (pageAccounts, page) =>
          api.listCurrentUserResources<BlueprintsListResponse>(
            "blueprints",
            pageAccounts,
            page,
          ),
        (current, page) => {
          const agents = [...(current?.agents ?? []), ...page.agents];
          return {
            ...page,
            agents,
            limit: agents.length,
            offset: 0,
            has_more: false,
          };
        },
        (page) => !!page.has_more,
      ),
    [api],
  );
  const query = useAllAccountResource({
    resource: "blueprints",
    enabled,
    accountFilters,
    load,
  });
  const blueprints = useMemo(
    () => query.data?.results.flatMap((result) => result.data.agents) ?? [],
    [query.data],
  );
  return { ...query, blueprints };
}

export function useAllAccountsKnowledgeStores(
  enabled = true,
  accountFilters?: string[],
) {
  const api = useApiClient();
  const load = useCallback(
    (accounts: string[]) =>
      loadCompleteAccountPages(
        accounts,
        (pageAccounts, page) =>
          api.listCurrentUserResources<KnowledgeStoreListResponse>(
            "knowledge",
            pageAccounts,
            page,
          ),
        (current, page) => [...(current ?? []), ...page],
        (page) => page.length >= AGGREGATE_PAGE_SIZE,
      ),
    [api],
  );
  const query = useAllAccountResource<KnowledgeStoreListResponse>({
    resource: "knowledge",
    enabled,
    accountFilters,
    load,
    isTransitional: (stores) =>
      stores.some((store) => TRANSITIONAL_KNOWLEDGE.includes(store.status)),
  });
  const stores = useMemo(
    () =>
      (query.data?.results ?? [])
        .flatMap((result) =>
          result.data.map((store) => ({ ...store, account: result.account })),
        )
        .sort(
          (a, b) =>
            b.created_at.localeCompare(a.created_at) || a.name.localeCompare(b.name),
        ),
    [query.data],
  );
  return { ...query, stores };
}

export function useAllAccountsDeployments(
  enabled = true,
  accountFilters?: string[],
) {
  const api = useApiClient();
  const load = useCallback(
    (accounts: string[]) =>
      loadCompleteAccountPages(
        accounts,
        (pageAccounts, page) =>
          api.listCurrentUserResources<DeploymentsListResponse>(
            "deployments",
            pageAccounts,
            page,
          ),
        (current, page) => {
          const deployments = new Map(
            current?.deployments.map((deployment) => [deployment.id, deployment]),
          );
          for (const deployment of page.deployments) {
            deployments.set(deployment.id, deployment);
          }
          return { ...page, deployments: [...deployments.values()] };
        },
        (page) => page.deployments.length >= AGGREGATE_PAGE_SIZE,
        (current, page) => {
          if (!current) return true;
          const ids = new Set(current.deployments.map((deployment) => deployment.id));
          return page.deployments.some((deployment) => !ids.has(deployment.id));
        },
      ),
    [api],
  );
  const query = useAllAccountResource<DeploymentsListResponse>({
    resource: "deployments",
    enabled,
    accountFilters,
    load,
    isTransitional: ({ deployments }) =>
      deployments.some((deployment) =>
        TRANSITIONAL_DEPLOYMENT.includes(deployment.status?.toLowerCase?.() ?? ""),
      ),
  });
  const deployments = useMemo(
    () =>
      (query.data?.results ?? [])
        .flatMap((result) =>
          result.data.deployments.map((deployment) => ({
            ...deployment,
            account: result.account,
          })),
        )
        .sort(
          (a, b) =>
            b.created_at.localeCompare(a.created_at) || a.name.localeCompare(b.name),
        ),
    [query.data],
  );
  return { ...query, deployments };
}
