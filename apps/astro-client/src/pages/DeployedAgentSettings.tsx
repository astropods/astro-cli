import { useParams, Link, useNavigate } from "react-router";
import type { Route } from "./+types/DeployedAgentSettings";
import { Loader2, Rocket, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";

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
  return [{ title: `Settings - ${name} | Astro` }];
};

function DeployedAgentSettingsContent({ loaderData }: { loaderData: Route.ComponentProps["loaderData"] }) {
  const { account: paramAccount, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const account = paramAccount ?? "";
  const navigate = useNavigate();
  const { isAuthenticated, personalAccount } = useAuth();

  const { data: deploymentsData } = useDeployments(account, isAuthenticated);

  const deployments = deploymentsData?.deployments ?? [];
  const deployment = deployments.find((d) => d.id === deploymentId) ?? loaderData?.deployment ?? null;

  // The form loads the template from the API using the agent's source name.
  // Initial values will be pre-filled from the deployed spec in a future change.
  const form = useDeployForm(account, deployment?.name ?? "", {
    initialValues: {
      deployName: deployment?.display_name || deployment?.name || "",
      targetAccount: account,
    },
  });

  const trackedState: TrackedFormState = {
    deployName: form.deployName,
    variableValues: form.variableValues,
    selectedAdapters: form.selectedAdapters,
    adapterCredentials: form.adapterCredentials,
  };
  const changes = useChangeTracking(trackedState);

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

  const displayName = deployment.display_name || deployment.name;
  const basePath = `/${account}/agents/${deployment.id}`;

  const isPersonal = personalAccount?.name === account;
  const breadcrumbItems = [
    isPersonal
      ? { label: "My Agents", to: "/agents" }
      : { label: account, to: `/${account}` },
    { label: displayName, to: basePath },
    { label: "Settings" },
  ];

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.trySubmit()) return;
    try {
      await form.deploy();
      navigate(basePath);
    } catch {
      // Error is captured in form.deployError
    }
  };

  return (
    <div className="flex flex-1 flex-col">
      <PageBreadcrumb items={breadcrumbItems} />

      <div className="flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="w-full max-w-xl mx-auto px-6 pt-10 pb-10 md:px-8">
          <DeployFormFields form={form} hideAccountPicker />
        </form>
      </div>

      {/* Floating action bar — slides in when changes are detected */}
      <div className="sticky bottom-0 z-10 flex justify-center pb-4 pointer-events-none">
        <div
          className={`w-full max-w-[calc(36rem+2rem)] mx-6 md:mx-8 border border-border rounded-lg bg-background shadow-lg pointer-events-auto transition-all duration-200 ${
            changes.isDirty
              ? "translate-y-0 opacity-100"
              : "translate-y-[calc(100%+1rem)] opacity-0"
          }`}
        >
          <div className="px-5 py-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="text-sm text-muted-foreground text-center sm:text-left">
              {changes.requiresRedeploy ? (
                <span>Changes require a redeploy</span>
              ) : (
                <span>No redeploy needed</span>
              )}
            </div>
            <div className="flex gap-3 sm:shrink-0">
              <Button
                type="button"
                variant="ghost"
                size="default"
                className="flex-1 sm:flex-none"
                asChild
              >
                <Link to={basePath}>Cancel</Link>
              </Button>
              <Button
                type="submit"
                size="default"
                disabled={form.isDeploying}
                onClick={handleSubmit}
                className="flex-1 sm:flex-none px-6 has-[>svg]:px-6"
              >
                {form.isDeploying ? (
                  <>
                    <Loader2 size={16} className="animate-spin" />
                    {changes.requiresRedeploy ? "Redeploying..." : "Saving..."}
                  </>
                ) : changes.requiresRedeploy ? (
                  <>
                    <Rocket size={16} />
                    Save &amp; Redeploy
                  </>
                ) : (
                  <>
                    <Save size={16} />
                    Save
                  </>
                )}
              </Button>
            </div>
          </div>
        </div>
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
