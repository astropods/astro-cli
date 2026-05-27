import { Outlet, useParams, useOutletContext, useLocation } from "react-router";
import { AnimatePresence, motion } from "motion/react";
import type { Route } from "./+types/AgentDetail";
import { createServerApi } from "@/lib/api.server";
import { PageStarField } from "@/components/agent-detail/starfield/PageStarField";
import { AgentTabBar } from "@/components/agent-detail/AgentTabBar";
import { AgentIdentity } from "@/components/agent-detail/AgentIdentity";
import { AgentStatusToggle } from "@/components/agent-detail/AgentStatusToggle";
import { useDeployment } from "@/api/queries/deployments";
import type { AgentDeployment } from "@/lib/api";
import { getMessagingEndpoint, isLaunchReady, launchUnavailableMessage } from "@/lib/deployment-utils";
import { ArrowUpRight } from "lucide-react";

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
  const deployment = data?.deployment ?? loaderData.deployment;
  const messagingEndpoint = getMessagingEndpoint(deployment);
  const messagingUrl = messagingEndpoint?.url;
  const launchReady = isLaunchReady(deployment);

  const context: AgentDetailContext | null = deployment
    ? { deployment, account: account ?? "", deploymentId: deploymentId ?? "" }
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
      {deployment && (
        <AgentIdentity
          account={account ?? ""}
          deployment={deployment}
        />
      )}
      <AgentTabBar />
      {deployment && (
        <div className="absolute top-4 right-0 z-20 flex items-center pr-6 max-[700px]:hidden">
          <div className="flex items-center gap-4 rounded-[8px] dark:rounded-md bg-background p-1 pl-3 pr-1 dark:bg-transparent dark:p-0 dark:pl-0 dark:pr-0">
            <AgentStatusToggle deployment={deployment} account={account ?? ""} />
            {messagingUrl && (
              <button
                type="button"
                className="flex items-center gap-2 rounded-sm bg-primary px-4 py-1.5 text-sm font-medium tracking-wide text-primary-foreground transition-opacity enabled:cursor-pointer enabled:hover:opacity-85 disabled:cursor-not-allowed disabled:opacity-50"
                disabled={!launchReady}
                title={!launchReady ? launchUnavailableMessage : undefined}
                onClick={() => window.open(messagingUrl, '_blank', 'noopener,noreferrer')}
              >
                Launch <ArrowUpRight className="size-3.5" />
              </button>
            )}
          </div>
        </div>
      )}
      <div className="relative z-10 flex min-h-0 flex-1">
        <Outlet context={context} />
      </div>
    </div>
    </div>
  );
}
