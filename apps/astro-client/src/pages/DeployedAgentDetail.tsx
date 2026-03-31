import { useEffect, useState } from "react";
import { useLocation, useParams, Link, Navigate } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import { useDeployment } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { isDeployingState } from "@/lib/deployment-utils";
import { Spinner } from "@/components/ui/spinner";

export function loader({ params }: Route.LoaderArgs) {
  return { account: params.account ?? "", deploymentId: params.deploymentId ?? "" };
}

export const meta: Route.MetaFunction = () => {
  return [{ title: "Agent | Astro" }];
};

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const location = useLocation();
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? loaderData?.account ?? "";
  const { isAuthenticated, personalAccount, isLoading: isAuthLoading, login } = useAuth();
  // isPending stays true for disabled queries (no data yet), covering the gap
  // between auth resolving and the query transitioning to its loading state.
  const { data: deploymentsData, isPending: isQueryPending } = useDeployment(deploymentId ?? "", isAuthenticated);
  const isLoading = isAuthLoading || (isAuthenticated && isQueryPending);
  const deployment = deploymentsData?.deployment ?? null;

  // If loading takes more than 2s (hung auth/network), show a spinner so
  // the user doesn't stare at a blank screen indefinitely.
  const [slowLoad, setSlowLoad] = useState(false);
  useEffect(() => {
    if (!isLoading) { setSlowLoad(false); return; }
    const t = setTimeout(() => setSlowLoad(true), 2000);
    return () => clearTimeout(t);
  }, [isLoading]);

  useEffect(() => {
    if (!isAuthLoading && !isAuthenticated) login();
  }, [isAuthLoading, isAuthenticated, login]);

  if (isLoading && slowLoad) {
    return (
      <div style={{ display: "flex", flex: 1, alignItems: "center", justifyContent: "center" }}>
        <Spinner size={20} />
      </div>
    );
  }

  if (isLoading) return null;

  if (!isAuthenticated) return null;

  if (!personalAccount) return <Navigate to="/onboarding" replace />;

  if (!deployment) {
    return (
      <div className="dp-fadein flex flex-col items-center justify-center py-16 px-6">
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

  const monitorLocked = isDeployingState(deployment);
  const isPersonal = personalAccount?.name === account;
  const requestedFromAgents = (location.state as { fromAgents?: boolean } | null)?.fromAgents === true;

  return (
    <div className="dp-fadein" style={{ display: "flex", flex: 1, flexDirection: "column", minHeight: 0 }}>
      <ActiveDetailView
        deployment={deployment}
        account={account}
        isPersonal={isPersonal}
        monitorLocked={monitorLocked}
        backPathOverride={requestedFromAgents ? "/agents" : undefined}
      />
    </div>
  );
}

export default function DeployedAgentDetail({ loaderData }: Route.ComponentProps) {
  return <DeployedAgentDetailContent loaderData={loaderData} />;
}
