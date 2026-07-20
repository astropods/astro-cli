import { useMemo } from "react";
import { Link, Outlet, useParams, useOutletContext, useLocation } from "react-router";
import { AnimatePresence, motion } from "motion/react";
import type { Route } from "./+types/AgentDetail";
import { createServerApi } from "@/lib/api.server";
import { PageStarField } from "@/components/agent-detail/starfield/PageStarField";
import { AgentTabBar } from "@/components/agent-detail/AgentTabBar";
import { AgentIdentity } from "@/components/agent-detail/AgentIdentity";
import { AgentStatusToggle } from "@/components/agent-detail/AgentStatusToggle";
import { useDeployment, useDeployments, useDeploymentRuntime, useDeploymentStatus } from "@/api/queries/deployments";
import type { AgentDeployment, DeploymentRuntime } from "@/lib/api";
import { chatDeploymentPath } from "@/lib/routes";
import { ArrowUpRight } from "lucide-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { getLaunchDisabledMessage, isChatListEligible, withLatestBuildId } from "@/lib/deployment-utils";

export async function loader({ params, request }: Route.LoaderArgs) {
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";
  const api = createServerApi(request);
  const deploymentData = deploymentId
    ? await api.getDeployment(deploymentId).catch(() => null)
    : null;
  return { account, deploymentId, deployment: deploymentData?.deployment ?? null };
}

/**
 * Prevent React Router from re-running the layout loader on child tab
 * navigations. The loader only needs to run on initial page load — after
 * that, TanStack Query handles data freshness via polling.
 */
export function shouldRevalidate({ currentUrl, nextUrl }: { currentUrl: URL; nextUrl: URL }) {
  // Compare up to /:account/agents/:deploymentId — exclude the tab segment
  const currentBase = currentUrl.pathname.split("/").slice(0, 4).join("/");
  const nextBase = nextUrl.pathname.split("/").slice(0, 4).join("/");
  return currentBase !== nextBase;
}

export const meta: Route.MetaFunction = () => {
  return [{ title: "Agent | Astro" }];
};

interface AgentDetailContext {
  deployment: AgentDeployment;
  // Runtime view stitched onto the record's workloads by name. Undefined
  // while the runtime query is still loading or if it errored — child tabs
  // must tolerate that without breaking the record-driven UI.
  runtime: DeploymentRuntime | undefined;
  account: string;
  deploymentId: string;
}

/** Hook for child routes to access the deployment data from the layout. */
export function useAgentDetailContext() {
  return useOutletContext<AgentDetailContext>();
}


export default function AgentDetail({ loaderData }: Route.ComponentProps) {
  const { account, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const location = useLocation();
  const isConfigureTab = location.pathname.endsWith("/configure");
  const { data } = useDeployment(deploymentId ?? "", true, {
    initialData: loaderData.deployment
      ? { deployment: loaderData.deployment }
      : undefined,
  });
  const { data: runtimeData } = useDeploymentRuntime(deploymentId ?? "");
  const { data: statusData } = useDeploymentStatus(deploymentId ?? "");
  const { data: deploymentsData } = useDeployments(account ?? "");
  const runtime = runtimeData?.runtime;
  const isActive = statusData?.value === "active";
  // Gate Launch on the web (chat) adapter — same signal the agents list and
  // chat surface use (messaging_web_configured), since Launch deep-links into
  // the web chat. The record endpoint only carries messaging_configured (true
  // even for slack-only agents), so read the summary from the list endpoint.
  const rawDeployment = data?.deployment ?? loaderData.deployment;
  const deploymentSummary = deploymentsData?.deployments.find((d) => d.id === deploymentId);
  const deployment = useMemo(
    () => withLatestBuildId(rawDeployment, deploymentSummary?.latest_build_id),
    [rawDeployment, deploymentSummary?.latest_build_id],
  );
  const canLaunch = isChatListEligible(deploymentSummary);
  const launchDisabled = !isActive;

  const context: AgentDetailContext | null = deployment
    ? { deployment, runtime, account: account ?? "", deploymentId: deploymentId ?? "" }
    : null;

  return (
    <div key={deploymentId} className="relative -mt-px min-h-0 flex-1">
    <div className="absolute inset-0 flex overflow-hidden">
      <PageStarField className="absolute inset-0" />
      {/* Softens the hard edge between the site header and the starfield */}
      <div
        className="pointer-events-none absolute inset-x-0 top-0 z-[1] h-10 dark:block hidden"
        style={{ background: "linear-gradient(to bottom, var(--color-background), transparent)" }}
      />
      <AnimatePresence>
        {isConfigureTab && (
          <motion.div
            className="pointer-events-none absolute inset-0 z-[1] bg-[linear-gradient(to_bottom,transparent_0%,var(--color-surface)_35%)] dark:bg-[linear-gradient(to_bottom,transparent_0%,var(--color-background)_35%)]"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.4, ease: "easeOut" }}
          />
        )}
      </AnimatePresence>
      <header className="pointer-events-none absolute inset-x-0 top-4 z-20 grid grid-cols-[minmax(0,1fr)_auto_minmax(max-content,1fr)] items-start gap-3 px-5 max-[700px]:grid-cols-[minmax(0,1fr)_auto] max-[700px]:px-3">
        <div className="min-w-0">
          {deployment && (
            <AgentIdentity
              account={account ?? ""}
              deployment={deployment}
            />
          )}
        </div>
        <AgentTabBar />
        {deployment && (
          <div className="pointer-events-auto flex items-center justify-self-end max-[700px]:hidden">
            <div className="flex items-center gap-4 rounded-[8px] bg-background p-1 pl-3 pr-1 dark:rounded-md dark:bg-transparent dark:p-0 dark:pl-0 dark:pr-0">
              <AgentStatusToggle deployment={deployment} account={account ?? ""} />
              {canLaunch && deploymentId && (
                <TooltipProvider delayDuration={0}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Button asChild={!launchDisabled} disabled={launchDisabled}>
                          {launchDisabled ? (
                            <>
                              Launch
                              <ArrowUpRight className="size-3.5" />
                            </>
                          ) : (
                            <Link to={chatDeploymentPath(deploymentId)}>
                              Launch
                              <ArrowUpRight className="size-3.5" />
                            </Link>
                          )}
                        </Button>
                      </span>
                    </TooltipTrigger>
                    {launchDisabled && (
                      <TooltipContent side="bottom" className="max-w-[240px] py-1.5" collisionPadding={8}>
                        {getLaunchDisabledMessage(statusData?.value)}
                      </TooltipContent>
                    )}
                  </Tooltip>
                </TooltipProvider>
              )}
            </div>
          </div>
        )}
      </header>
      <div className="relative z-10 flex min-h-0 flex-1">
        <Outlet context={context} />
      </div>
    </div>
    </div>
  );
}
