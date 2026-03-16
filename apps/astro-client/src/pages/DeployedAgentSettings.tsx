import { useMemo } from "react";
import { useParams, Link, Outlet } from "react-router";
import type { Route } from "./+types/DeployedAgentSettings";
import { Loader2 } from "lucide-react";
import { Rocket, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployments } from "@/api/queries/deployments";
import { useAgent, usePrefilledDeploymentTemplate } from "@/api/queries/agents";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { deploymentPath, deploymentConfigurePath } from "@/lib/routes";
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from "@/components/ui/sidebar-layout";

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";

  const deploymentsData = await api.listDeployments(account).catch(() => ({ deployments: [], count: 0 }));

  const deployment = deploymentsData.deployments.find((d) => d.id === deploymentId) ?? null;

  return { deployment, account, deploymentId };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const name = data?.deployment?.display_name || data?.deployment?.name || "Agent";
  return [{ title: `Configure - ${name} | Astro` }];
};

function DeployedAgentSettingsContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? "";
  const { isAuthenticated, personalAccount } = useAuth();

  const { data: deploymentsData } = useDeployments(account, isAuthenticated);

  const deployments = deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.id === deploymentId) ?? loaderData?.deployment ?? null;
  const { data: agentData } = useAgent(account, deployment?.name ?? "", {
    initialData: undefined,
  });

  const latestBuildId = useMemo(() => {
    if (!agentData?.versions?.length) return null;
    const latestVersion = agentData.versions.reduce((latest, current) => {
      if (!latest) return current;
      return new Date(current.published_at).getTime() > new Date(latest.published_at).getTime()
        ? current
        : latest;
    }, agentData.versions[0]);
    return latestVersion?.build_id ?? null;
  }, [agentData]);

  const hasNewerBuildAvailable = !!deployment?.build_id && !!latestBuildId && deployment.build_id !== latestBuildId;

  const {
    data: prefilledTemplate,
    isLoading: templateLoading,
    error: templateError,
  } = usePrefilledDeploymentTemplate(account, deployment?.name ?? "", deploymentId ?? "", {
    enabled: !!deployment?.name && !!deploymentId,
  });

  if (!deployment) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The deployed agent you&apos;re looking for doesn&apos;t exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/agents">My Agents</Link>
        </Button>
      </div>
    );
  }

  if (templateLoading || !prefilledTemplate) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <Loader2 size={24} className="animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (templateError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Failed to load settings</h1>
        <p className="text-muted-foreground text-sm mb-4">
          Could not load deployment configuration. Please try again.
        </p>
        <Button asChild>
          <Link to={deploymentPath(account, deployment.id)}>Back to agent</Link>
        </Button>
      </div>
    );
  }

  const displayName = deployment.display_name || deployment.name;
  const basePath = deploymentPath(account, deployment.id);
  const configurePath = deploymentConfigurePath(account, deployment.id);

  const isPersonal = personalAccount?.name === account;
  const breadcrumbItems = [
    isPersonal
      ? { label: "My Agents", to: "/agents" }
      : { label: account, to: `/${account}` },
    { label: displayName, to: basePath },
    { label: "Configure" },
  ];

  return (
    <div className="flex flex-1 flex-col">
      <PageBreadcrumb items={breadcrumbItems} />

      <div className="flex-1 w-full max-w-3xl mx-auto px-6 pt-8 pb-10 md:px-8">
        <SidebarLayout>
          <SidebarNav label="Configure">
            <SidebarNavItem to={`${configurePath}/deployment`}>
              <span className="flex items-center gap-2">
                <Rocket className="size-3.5" />
                Deployment
              </span>
            </SidebarNavItem>
            <SidebarNavItem to={`${configurePath}/danger-zone`}>
              <span className="flex items-center gap-2">
                <AlertTriangle className="size-3.5" />
                Danger Zone
              </span>
            </SidebarNavItem>
          </SidebarNav>
          <SidebarBody>
            <Outlet context={{ account, deployment, template: prefilledTemplate, hasNewerBuildAvailable }} />
          </SidebarBody>
        </SidebarLayout>
      </div>
    </div>
  );
}

export default function DeployedAgentSettings({ loaderData }: Route.ComponentProps) {
  return (
    <ProtectedRoute>
      <DeployedAgentSettingsContent loaderData={loaderData} />
    </ProtectedRoute>
  );
}
