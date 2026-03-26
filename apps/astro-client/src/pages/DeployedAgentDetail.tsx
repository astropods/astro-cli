import { useEffect, useRef, useState } from "react";
import { useLocation, useNavigate, useParams, Link } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import { LiveRevealOverlay } from "@/components/deployed-agent/detail/LiveRevealOverlay";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { isDeployingState, mapDeploymentStatus } from "@/lib/deployment-utils";


export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";

  const deploymentsData = await api.listDeployments(account).catch(() => ({ deployments: [], count: 0 }));
  const deployment = deploymentsData.deployments?.find((d) => d.id === deploymentId) ?? null;

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
  const location = useLocation();
  const navigate = useNavigate();
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? "";
  const { isAuthenticated, personalAccount } = useAuth();
  const [showLiveReveal, setShowLiveReveal] = useState(false);
  const [hasSeenReveal, setHasSeenReveal] = useState(false);
  const [hasLoadedRevealSeen, setHasLoadedRevealSeen] = useState(false);
  const [stayOnDeployments, setStayOnDeployments] = useState(false);
  const [allowMonitorTab, setAllowMonitorTab] = useState(false);
  const trackedDeploymentIdRef = useRef<string | null>(null);

  const { data: deploymentsData } = useDeployments(account, isAuthenticated);
  const deployments = deploymentsData?.deployments ?? loaderData?.deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.id === deploymentId) ?? loaderData?.deployment ?? null;
  const currentDeploymentId = deployment?.id ?? null;
  const status = deployment ? mapDeploymentStatus(deployment) : null;
  const monitorLocked = deployment ? isDeployingState(deployment) : false;
  const isPersonal = personalAccount?.name === account;
  const queryTab = new URLSearchParams(location.search).get("tab");
  const queryFrom = new URLSearchParams(location.search).get("from");
  const queryReveal = new URLSearchParams(location.search).get("reveal");
  const requestedTab = queryTab === "monitor" || queryTab === "deployments" ? queryTab : null;
  const requestedFromAgents = queryFrom === "agents";
  const requestedFirstDeployReveal = queryReveal === "first-deploy";
  const initialTab: "monitor" | "deployments" =
    (monitorLocked || stayOnDeployments)
      ? "deployments"
      : (requestedTab === "monitor" && (allowMonitorTab || requestedFromAgents) ? "monitor" : "deployments");
  const revealSeenKey = deployment
    ? `astro:deploy-live-reveal:${account}:${deployment.name}:${deployment.id}`
    : "";

  useEffect(() => {
    if (!currentDeploymentId) return;
    if (trackedDeploymentIdRef.current !== currentDeploymentId) {
      setAllowMonitorTab(false);
    }
    trackedDeploymentIdRef.current = currentDeploymentId;
    setShowLiveReveal(false);
    setStayOnDeployments(false);
    setHasLoadedRevealSeen(false);

    const revealSeen = typeof window !== "undefined" && window.localStorage.getItem(revealSeenKey) === "1";
    setHasSeenReveal(revealSeen);
    setHasLoadedRevealSeen(true);
  }, [currentDeploymentId, revealSeenKey]);

  useEffect(() => {
    if (!deployment || !status) return;
    if (!hasLoadedRevealSeen) return;
    if (trackedDeploymentIdRef.current !== deployment.id) return;
    if (requestedFirstDeployReveal && showLiveReveal) return;

    if (requestedFirstDeployReveal && !hasSeenReveal && (status === "pending" || status === "active")) {
      setStayOnDeployments(true);
      setShowLiveReveal(true);
      setHasSeenReveal(true);
      if (typeof window !== "undefined") {
        window.localStorage.setItem(revealSeenKey, "1");
      }
      return;
    }

    if (status === "pending") {
      setStayOnDeployments(true);
      setShowLiveReveal(false);
      return;
    }
    setShowLiveReveal(false);
  }, [deployment, hasLoadedRevealSeen, hasSeenReveal, requestedFirstDeployReveal, revealSeenKey, status]);

  useEffect(() => {
    if (requestedTab !== "monitor" || allowMonitorTab || requestedFromAgents) return;
    const params = new URLSearchParams(location.search);
    params.delete("tab");
    const next = params.toString();
    navigate(`${location.pathname}${next ? `?${next}` : ""}`, { replace: true });
  }, [allowMonitorTab, location.pathname, location.search, navigate, requestedFromAgents, requestedTab]);

  if (!deployment) {
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

  const backgroundDeployment = showLiveReveal
    ? { ...deployment, status: "pending", ready: 0 }
    : deployment;
  const backgroundInitialTab: "monitor" | "deployments" = showLiveReveal ? "deployments" : initialTab;
  const backgroundMonitorLocked = showLiveReveal ? true : monitorLocked;

  return (
    <>
      <ActiveDetailView
        key={`${deployment.id}-${backgroundInitialTab}-${showLiveReveal ? "reveal" : "normal"}`}
        deployment={backgroundDeployment}
        account={account}
        isPersonal={isPersonal}
        initialTab={backgroundInitialTab}
        monitorLocked={backgroundMonitorLocked}
        backPathOverride={requestedFromAgents ? "/agents" : undefined}
      />
      {showLiveReveal && (
        <LiveRevealOverlay
          deployment={deployment}
          account={account}
          onDismiss={() => {
            setStayOnDeployments(true);
            const params = new URLSearchParams(location.search);
            params.delete("reveal");
            navigate(`${location.pathname}${params.toString() ? `?${params.toString()}` : ""}`, { replace: true });
            setShowLiveReveal(false);
          }}
          onViewDeployment={() => {
            setStayOnDeployments(true);
            const params = new URLSearchParams(location.search);
            params.delete("reveal");
            navigate(`${location.pathname}${params.toString() ? `?${params.toString()}` : ""}`, { replace: true });
            setShowLiveReveal(false);
          }}
        />
      )}
    </>
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
