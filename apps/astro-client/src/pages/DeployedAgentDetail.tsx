import { useState, useEffect, useRef } from "react";
import { useParams, Link, useLocation } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { DeployingDetailView } from "@/components/deployed-agent/detail/DeployingDetailView";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import { LiveRevealOverlay } from "@/components/deployed-agent/detail/LiveRevealOverlay";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { mapDeploymentStatus } from "@/lib/deployment-utils";
import type { UILifecycleState } from "@/lib/deployment-utils";
import { useQueryClient } from "@tanstack/react-query";
import { deploymentKeys } from "@/api/queries/keys";


export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";

  const deploymentsData = await api.listDeployments(account).catch(() => ({ deployments: [], count: 0 }));
  const deployment = deploymentsData.deployments.find((d) => d.id === deploymentId || d.name === deploymentId) ?? null;

  return { deploymentsData, deployment, account, deploymentId };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const name = data?.deployment?.display_name || data?.deployment?.name || "Agent";
  return [{ title: `${name} | Astro` }];
};

function DeployedAgentDetailSkeleton() {
  return (
    <div className="flex flex-1 flex-col">
      <div className="flex items-center justify-between px-6 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-3.5 w-3.5" />
          <Skeleton className="h-4 w-32" />
        </div>
      </div>
      <div className="mx-auto w-full max-w-3xl">
        <div className="flex items-center gap-4 px-6 py-6">
          <Skeleton className="size-14 rounded-lg" />
          <div className="space-y-2">
            <Skeleton className="h-6 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
        </div>
      </div>
    </div>
  );
}

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? "";
  const { isAuthenticated, personalAccount } = useAuth();
  const location = useLocation();
  const queryClient = useQueryClient();
  const fromDeploy = (location.state as { fromDeploy?: boolean } | null)?.fromDeploy === true;

  // Derive initial lifecycle state from loader data
  const loaderDeployment =
    loaderData?.deploymentsData?.deployments.find((d) => d.id === deploymentId) ??
    loaderData?.deployment ??
    null;
  const initialStatus = loaderDeployment ? mapDeploymentStatus(loaderDeployment) : null;

  // If navigated here from a deploy action, always start in pending state so the
  // deploying view is shown even when K8s still reports the previous running state.
  const [lifecycleState, setLifecycleState] = useState<UILifecycleState>(
    fromDeploy || initialStatus === 'pending' ? 'pending' : (initialStatus ?? 'active'),
  );

  // Track whether we entered from pending — only show live-reveal if we transitioned from pending
  const enteredFromPending = useRef(fromDeploy || initialStatus === 'pending');
  // Guard to only trigger live-reveal once per mount
  const liveRevealTriggered = useRef(false);

  // When arriving from a deploy action, immediately invalidate the stale cache so
  // the first poll returns fresh data rather than a cached pre-deploy snapshot.
  useEffect(() => {
    if (fromDeploy) {
      queryClient.invalidateQueries({ queryKey: deploymentKeys.all(account) });
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const { data: deploymentsData } = useDeployments(account, isAuthenticated, {
    // Poll every 2s while deploying for responsive stage progression
    refetchInterval: lifecycleState === 'pending' ? 2000 : false,
  });

  const deployments = deploymentsData?.deployments ?? loaderData?.deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.id === deploymentId || d.name === deploymentId) ?? loaderData?.deployment ?? null;

  // Watch deployment status changes to transition lifecycle state
  useEffect(() => {
    if (!deployment) return;
    const status = mapDeploymentStatus(deployment);

    if (lifecycleState === 'pending') {
      if (status === 'active') {
        if (enteredFromPending.current && !liveRevealTriggered.current) {
          liveRevealTriggered.current = true;
          // Brief pause so the user sees all stages flip to checkmarks before the overlay
          setTimeout(() => setLifecycleState('live-reveal'), 900);
        } else {
          setLifecycleState('active');
        }
      } else if (status === 'error') {
        setLifecycleState('error');
      }
      // Never transition pending → inactive: replicas=0 is transient during K8s startup.
    }
  }, [deployment?.status, deployment?.ready, deployment?.replicas]);

  // While in a fresh deploy flow and the deployment hasn't appeared yet, keep
  // polling and show a loading state rather than a "not found" error page.
  if (!deployment) {
    if (fromDeploy || lifecycleState === 'pending') {
      return (
        <div style={{ display: 'flex', flex: 1, alignItems: 'center', justifyContent: 'center', background: '#ede7d9' }}>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 16 }}>
            <svg style={{ animation: 'spin 1s linear infinite' }} width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#15827d" strokeWidth="2" strokeLinecap="round">
              <path d="M21 12a9 9 0 1 1-6.219-8.56" />
            </svg>
            <style>{`@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }`}</style>
            <p style={{ fontFamily: "'Geist Mono', monospace", fontSize: 12, color: '#6b7e7c', letterSpacing: '0.06em' }}>Starting deployment…</p>
          </div>
        </div>
      );
    }
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The deployed agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/agents">My Agents</Link>
        </Button>
      </div>
    );
  }

  const isPersonal = personalAccount?.name === account;

  if (lifecycleState === 'live-reveal') {
    return (
      <LiveRevealOverlay
        deployment={deployment}
        account={account}
        onComplete={() => setLifecycleState('active')}
      />
    );
  }

  if (lifecycleState === 'pending') {
    return (
      <DeployingDetailView
        deployment={deployment}
        account={account}
        isPersonal={isPersonal}
      />
    );
  }

  // active | inactive | error
  return (
    <ActiveDetailView
      deployment={deployment}
      account={account}
      isPersonal={isPersonal}
      onRedeploy={() => {
        enteredFromPending.current = true;
        liveRevealTriggered.current = false;
        setLifecycleState('pending');
      }}
    />
  );
}

export default function DeployedAgentDetail({ loaderData }: Route.ComponentProps) {
  if (!loaderData) {
    return <DeployedAgentDetailSkeleton />;
  }

  return (
    <ProtectedRoute>
      <DeployedAgentDetailContent loaderData={loaderData} />
    </ProtectedRoute>
  );
}
