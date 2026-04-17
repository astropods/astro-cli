import { Suspense } from "react";
import { useLocation, useParams, Link } from "react-router";
import type { Route } from "./+types/DeployedAgentDetail";
import { Button } from "@/components/ui/button";
import { ActiveDetailView } from "@/components/deployed-agent/detail/ActiveDetailView";
import { useDeploymentSuspense } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { isDeployingState } from "@/lib/deployment-utils";
import { dashboardPath } from "@/lib/routes";
import { Spinner } from "@/components/ui/spinner";

export function loader({ params }: Route.LoaderArgs) {
  return { account: params.account ?? "", deploymentId: params.deploymentId ?? "" };
}

export const meta: Route.MetaFunction = () => {
  return [{ title: "Agent | Astro" }];
};

function SpinnerFallback() {
  return (
    <div className="flex flex-1 items-center justify-center">
      <Spinner size={20} delay={2000} />
    </div>
  );
}

// Inner component — only rendered after auth is confirmed, inside a Suspense
// boundary. useSuspenseQuery throws while the deployment is loading so the
// Suspense fallback (delayed Spinner) handles the waiting state.
function DeployedAgentDetailData({ deploymentId, account, personalAccount }: {
  deploymentId: string;
  account: string;
  personalAccount: NonNullable<ReturnType<typeof useAuth>["personalAccount"]>;
}) {
  const location = useLocation();
  const { data } = useDeploymentSuspense(deploymentId);
  const deployment = data?.deployment ?? null;

  if (!deployment) {
    return (
      <div className="dp-fadein flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The deployed agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to={dashboardPath}>Agents</Link>
        </Button>
      </div>
    );
  }

  const monitorLocked = isDeployingState(deployment);
  const isPersonal = personalAccount.name === account;
  const state = location.state as { fromAgents?: boolean; backPath?: string } | null;
  const requestedFromAgents = state?.fromAgents === true;
  const backPath = state?.backPath;

  return (
    <div className="dp-fadein flex flex-col h-[calc(100vh-56px)] overflow-hidden">
      <ActiveDetailView
        deployment={deployment}
        account={account}
        isPersonal={isPersonal}
        monitorLocked={monitorLocked}
        backPathOverride={requestedFromAgents ? (backPath ?? dashboardPath) : undefined}
      />
    </div>
  );
}

function DeployedAgentDetailContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? loaderData?.account ?? "";
  const { personalAccount } = useAuth();

  return (
    <Suspense fallback={<SpinnerFallback />}>
      <DeployedAgentDetailData
        deploymentId={deploymentId ?? ""}
        account={account}
        personalAccount={personalAccount!}
      />
    </Suspense>
  );
}

export default function DeployedAgentDetail({ loaderData }: Route.ComponentProps) {
  return <DeployedAgentDetailContent loaderData={loaderData} />;
}
