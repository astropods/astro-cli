import { useMemo } from "react";
import { useParams, Link, Outlet } from "react-router";
import type { Route } from "./+types/DeployedAgentSettings";
import { Loader2 } from "lucide-react";
import { Rocket, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployments } from "@/api/queries/deployments";
import { useBlueprint, usePrefilledDeploymentTemplate } from "@/api/queries/blueprints";
import { useAuth } from "@/lib/auth";
import { deploymentPath, deploymentConfigurePath } from "@/lib/routes";
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from "@/components/ui/sidebar-layout";


export async function loader({ params }: Route.LoaderArgs) {
  const account = params.account ?? "";
  const deploymentId = params.deploymentId ?? "";

  return { account, deploymentId };
}

export const meta: Route.MetaFunction = () => {
  return [{ title: "Configure - Agent | Astro" }];
};

function DeployedAgentSettingsContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? loaderData?.account ?? "";
  const { isAuthenticated, personalAccount } = useAuth();

  const { data: deploymentsData, isLoading } = useDeployments(account, isAuthenticated);

  const deployments = deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.id === deploymentId) ?? null;
  const { data: blueprintData } = useBlueprint(account, deployment?.name ?? "", {
    initialData: undefined,
  });

  const latestBuildId = useMemo(() => {
    if (!blueprintData?.versions?.length) return null;
    const latestVersion = blueprintData.versions.reduce((latest, current) => {
      if (!latest) return current;
      return new Date(current.published_at).getTime() > new Date(latest.published_at).getTime()
        ? current
        : latest;
    }, blueprintData.versions[0]);
    return latestVersion?.build_id ?? null;
  }, [blueprintData]);

  const hasNewerBuildAvailable = !!deployment?.build_id && !!latestBuildId && deployment.build_id !== latestBuildId;

  const {
    data: prefilledTemplate,
    isLoading: templateLoading,
    error: templateError,
  } = usePrefilledDeploymentTemplate(account, deployment?.name ?? "", deploymentId ?? "", {
    enabled: !!deployment?.name && !!deploymentId,
  });

  if (isLoading || !deployment) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        {isLoading ? (
          <Loader2 size={24} className="animate-spin text-muted-foreground" />
        ) : (
          <>
            <h1 className="text-xl font-semibold mb-3">Deployment not found</h1>
            <p className="text-muted-foreground text-sm mb-4">
              The deployed agent you&apos;re looking for doesn&apos;t exist or has been removed.
            </p>
            <Button asChild>
              <Link to="/agents">My Agents</Link>
            </Button>
          </>
        )}
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

      <div className="flex-1 w-full max-w-3xl mx-auto px-6 pt-8 pb-10 md:px-8 md:overflow-hidden">
        <SidebarLayout className="md:h-full">
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
          <SidebarBody className="md:overflow-y-auto">
            <Outlet
              context={{
                account,
                deployment,
                template: prefilledTemplate,
                hasNewerBuildAvailable,
                currentBuildId: deployment.build_id,
                latestBuildId: latestBuildId ?? undefined,
              }}
            />
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
