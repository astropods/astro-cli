import { useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Route } from "./+types/AgentDashboard";
import { loadUserResourceScoped } from "@/lib/api.server";
import { DeployedAgentsSection } from "@/components/dashboard/DeployedAgentsSection";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { USER_DEPLOYMENTS_PAGE_SIZE, useUserDeployments } from "@/api/queries/deployments";
import { deploymentKeys } from "@/api/queries/keys";
import { useAuth } from "@/lib/auth";
import { useActiveAccount } from "@/hooks/use-active-account";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { firstInfinitePage } from "@/hooks/use-prime-query-cache";
import { useAccountFilterParam } from "@/hooks/use-account-filter-param";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useUserResourceSearch } from "@/hooks/use-user-resource-search";
import { resolveUserResourceScope } from "@/lib/user-resource-scope";
import { deploymentPath } from "@/lib/routes";
import { LiveRevealOverlay } from "@/components/ui/LiveRevealOverlay";
import type { AgentDeploymentSummary, AvatarColors } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Agents | Astro" }];

// Inline (not loadAccountScoped) to prime the deployments cache before render.
export async function loader({ request }: Route.LoaderArgs) {
  return loadUserResourceScoped(request, (api, scope) =>
    api.listUserDeployments(scope, { limit: USER_DEPLOYMENTS_PAGE_SIZE }),
  );
}


function AgentDashboardInner() {
  const { accounts, isAuthenticated } = useAuth();
  const { activeAccount: userAccount } = useActiveAccount();
  const location = useLocation();
  const navigate = useNavigate();

  const [revealDeployment] = useState<AgentDeploymentSummary | null>(() => {
    const rs = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAvatarColors?: AvatarColors } | null;
    if (!rs?.revealDeploymentId || !rs.revealAgentName) return null;
    return {
      id: rs.revealDeploymentId,
      name: rs.revealAgentName,
      display_name: rs.revealDisplayName ?? rs.revealAgentName,
      avatar_colors: rs.revealAvatarColors,
      build_id: "",
      created_at: new Date().toISOString(),
    } satisfies AgentDeploymentSummary;
  });
  const [showReveal, setShowReveal] = useState(!!revealDeployment);
  const [accountFilters, setAccountFilters] = useAccountFilterParam();
  const scope = useMemo(
    () => resolveUserResourceScope(accountFilters, accounts.map((account) => account.name)),
    [accountFilters, accounts],
  );
  const { search, setSearch, params } = useUserResourceSearch();
  const deploymentsQuery = useUserDeployments(scope, params, isAuthenticated);
  const deploymentPages = deploymentsQuery.data?.pages ?? [];
  const pagination = useCursorPagination({
    pages: deploymentPages,
    hasNextPage: !!deploymentsQuery.hasNextPage,
    isFetchingNextPage: deploymentsQuery.isFetchingNextPage,
    fetchNextPage: deploymentsQuery.fetchNextPage,
    resetKey: JSON.stringify([scope.all, scope.accounts, params]),
  });
  const deployments = pagination.page?.deployments ?? [];


  const clearRevealState = () => {
    navigate(location.pathname + location.search, { replace: true, state: {} });
  };

  return (
    <>
      <PageContainer
        outerClassName="bg-background"
      >
        <PageHeader
          title="Agents"
          description="Deployed agents running across your accounts."
        />

        <DeployedAgentsSection
          deployments={deployments}
          account={userAccount}
          isLoading={deploymentsQuery.isPending}
          isError={deploymentsQuery.isError}
          onRetry={() => void deploymentsQuery.refetch()}
          accountFilters={accountFilters}
          onAccountFiltersChange={setAccountFilters}
          search={search}
          onSearchChange={setSearch}
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          isChangingPage={deploymentsQuery.isFetchingNextPage}
          onPageChange={pagination.onPageChange}
          skeletonDeploymentId={showReveal ? revealDeployment?.id ?? null : null}
        />
      </PageContainer>

      {showReveal && revealDeployment && (
        <LiveRevealOverlay
          deployment={revealDeployment}
          account={userAccount}
          onDismiss={() => {
            setShowReveal(false);
            clearRevealState();
          }}
          onViewDeployment={() => {
            const targetPath = deploymentPath(userAccount, revealDeployment.id);
            const backPath = location.pathname + location.search;
            window.history.replaceState({}, "", location.pathname + location.search);
            navigate(targetPath, { state: { fromAgents: true, backPath } });
          }}
        />
      )}
    </>
  );
}

export default function AgentDashboard({ loaderData }: Route.ComponentProps) {
  usePrimeQueryCache(loaderData, (qc, ld) => {
    if (!ld?.scope || !ld.data) return;
    qc.setQueryData(
      deploymentKeys.visibleList(ld.scope, { limit: USER_DEPLOYMENTS_PAGE_SIZE }),
      firstInfinitePage(ld.data),
    );
  });

  return <AgentDashboardInner />;
}
