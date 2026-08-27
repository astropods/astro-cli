import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Route } from "./+types/AgentDashboard";
import { loadOrgScoped } from "@/lib/api.server";
import { DeployedAgentsSection } from "@/components/dashboard/DeployedAgentsSection";
import { AccountScopeFilter } from "@/components/AccountScopeFilter";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { USER_DEPLOYMENTS_PAGE_SIZE, useUserDeployments } from "@/api/queries/deployments";
import { deploymentKeys } from "@/api/queries/keys";
import { useAuth } from "@/lib/auth";
import { useOrgScope } from "@/hooks/use-org-scope";
import { usePrimeQueryCache } from "@/hooks/use-prime-query-cache";
import { firstInfinitePage } from "@/hooks/use-prime-query-cache";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useUserResourceSearch } from "@/hooks/use-user-resource-search";
import { shouldRevalidateUserResourceList } from "@/lib/user-resource-revalidation";
import { deploymentPath } from "@/lib/routes";
import { LiveRevealOverlay } from "@/components/ui/LiveRevealOverlay";
import type { AgentDeploymentSummary, AvatarColors } from "@/lib/api";

export const meta: Route.MetaFunction = () => [{ title: "Agents | Astro" }];
export const shouldRevalidate = shouldRevalidateUserResourceList;

const ACCESS_PROVISIONING_DELAYED_MS = 10_000;
const ACCESS_PROVISIONING_STALLED_MS = 120_000;

export async function loader({ request }: Route.LoaderArgs) {
  return loadOrgScoped(request, (api, scope) =>
    api.listUserDeployments(scope, { limit: USER_DEPLOYMENTS_PAGE_SIZE }),
  );
}


function AgentDashboardInner() {
  const { isAuthenticated } = useAuth();
  const { account: userAccount, setAccount, scope } = useOrgScope();
  const location = useLocation();
  const navigate = useNavigate();

  const [revealDeployment] = useState<AgentDeploymentSummary | null>(() => {
    const rs = location.state as { revealDeploymentId?: string; revealAgentName?: string; revealDisplayName?: string; revealAccount?: string; revealAvatarColors?: AvatarColors } | null;
    if (!rs?.revealDeploymentId || !rs.revealAgentName) return null;
    return {
      id: rs.revealDeploymentId,
      name: rs.revealAgentName,
      display_name: rs.revealDisplayName ?? rs.revealAgentName,
      account_name: rs.revealAccount,
      avatar_colors: rs.revealAvatarColors,
      build_id: "",
      created_at: new Date().toISOString(),
    } satisfies AgentDeploymentSummary;
  });
  const [showReveal, setShowReveal] = useState(!!revealDeployment);
  const { search, setSearch, params } = useUserResourceSearch();
  const [accessProvisioningDelayed, setAccessProvisioningDelayed] = useState(false);
  const [accessProvisioningStalled, setAccessProvisioningStalled] = useState(false);
  const [accessProvisioningAttempt, setAccessProvisioningAttempt] = useState(0);
  const deploymentsQuery = useUserDeployments(scope, params, isAuthenticated, {
    pendingDeploymentId: revealDeployment?.id,
    pendingAccessPollInterval: accessProvisioningStalled
      ? false
      : accessProvisioningDelayed
        ? 10_000
        : 2000,
  });
  const deploymentPages = deploymentsQuery.data?.pages ?? [];
  const pagination = useCursorPagination({
    pages: deploymentPages,
    hasNextPage: !!deploymentsQuery.hasNextPage,
    isFetchingNextPage: deploymentsQuery.isFetchingNextPage,
    fetchNextPage: deploymentsQuery.fetchNextPage,
    resetKey: JSON.stringify([scope.all, scope.accounts, params]),
  });
  const listedDeployments = pagination.page?.deployments ?? [];
  const resolvedRevealDeployment = revealDeployment
    ? deploymentPages.flatMap((page) => page.deployments).find((deployment) => deployment.id === revealDeployment.id)
    : undefined;
  const accessProvisioning = !!revealDeployment
    && (!resolvedRevealDeployment || resolvedRevealDeployment.access_ready === false);
  useEffect(() => {
    if (!accessProvisioning) {
      setAccessProvisioningDelayed(false);
      setAccessProvisioningStalled(false);
      return;
    }
    setAccessProvisioningDelayed(false);
    setAccessProvisioningStalled(false);
    const delayedTimeout = window.setTimeout(
      () => setAccessProvisioningDelayed(true),
      ACCESS_PROVISIONING_DELAYED_MS,
    );
    const stalledTimeout = window.setTimeout(
      () => setAccessProvisioningStalled(true),
      ACCESS_PROVISIONING_STALLED_MS,
    );
    return () => {
      window.clearTimeout(delayedTimeout);
      window.clearTimeout(stalledTimeout);
    };
  }, [accessProvisioning, accessProvisioningAttempt]);

  const revealAccount = revealDeployment?.account_name || userAccount;
  const revealQuery = params.q?.toLowerCase() ?? "";
  const revealMatchesScope = scope.accounts.includes(revealAccount);
  const revealMatchesSearch =
    !revealQuery ||
    (!!revealDeployment &&
      (revealDeployment.name.toLowerCase().includes(revealQuery) ||
        revealDeployment.display_name?.toLowerCase().includes(revealQuery)));
  const showSyntheticReveal =
    !resolvedRevealDeployment &&
    !!revealDeployment &&
    pagination.currentPage === 1 &&
    revealMatchesScope &&
    revealMatchesSearch;
  const deployments = showSyntheticReveal && revealDeployment
    ? [revealDeployment, ...listedDeployments]
    : listedDeployments;

  const retryAccessProvisioning = () => {
    setAccessProvisioningDelayed(false);
    setAccessProvisioningStalled(false);
    setAccessProvisioningAttempt((attempt) => attempt + 1);
    void deploymentsQuery.refetch();
  };

  const clearRevealState = () => {
    navigate(location.pathname + location.search, { replace: true, state: {} });
  };

  return (
    <>
      <PageContainer
        outerClassName="bg-background"
      >
        <PageHeader
          title="Agents for"
          adornment={<AccountScopeFilter value={userAccount} onChange={setAccount} className="-ml-1" />}
          description="Deployed agents running in this account."
        />

        <DeployedAgentsSection
          deployments={deployments}
          account={userAccount}
          isLoading={deploymentsQuery.isPending}
          isError={deploymentsQuery.isError}
          onRetry={() => void deploymentsQuery.refetch()}
          search={search}
          onSearchChange={setSearch}
          currentPage={pagination.currentPage}
          totalPages={pagination.totalPages}
          isChangingPage={deploymentsQuery.isFetchingNextPage}
          onPageChange={pagination.onPageChange}
          resultsTransitionKey={JSON.stringify([
            scope.all,
            scope.accounts,
            params,
            pagination.currentPage,
            deploymentsQuery.isPending,
          ])}
          accessProvisioningDeploymentId={accessProvisioning ? revealDeployment?.id ?? null : null}
          accessProvisioningDelayed={accessProvisioningDelayed}
          accessProvisioningStalled={accessProvisioningStalled}
          onRetryAccessProvisioning={retryAccessProvisioning}
        />
      </PageContainer>

      {showReveal && revealDeployment && (
        <LiveRevealOverlay
          deployment={revealDeployment}
          account={revealDeployment.account_name || userAccount}
          accessReady={!accessProvisioning}
          accessDelayed={accessProvisioningDelayed}
          accessStalled={accessProvisioningStalled}
          deploymentStatus={resolvedRevealDeployment?.status}
          onRetryAccess={retryAccessProvisioning}
          onDismiss={() => {
            setShowReveal(false);
            clearRevealState();
          }}
          onViewDeployment={() => {
            if (accessProvisioning) return;
            const targetPath = deploymentPath(revealDeployment.account_name || userAccount, revealDeployment.id);
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
