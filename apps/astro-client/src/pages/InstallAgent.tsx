import { useParams, Link, useNavigate } from "react-router";
import type { Route } from "./+types/InstallAgent";
import { Loader2, Rocket } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAgent } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { InterfacesPicker } from "@/components/deploy/InterfacesPicker";
import { VariableFields } from "@/components/deploy/VariableFields";
import { FormSection } from "@/components/deploy/FormSection";
import { ErrorPanel } from "@/components/deploy/ErrorPanel";
import { PageBreadcrumb } from "@/components/PageBreadcrumb";
import { AgentIdentity } from "@/components/AgentIdentity";

// --- Loader & Meta ---

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";

  if (!account || !agentSlug) {
    return { agent: null, template: null };
  }

  const [agent, template] = await Promise.all([
    api.getAgent(account, agentSlug).catch(() => null),
    api.getDeploymentTemplate(account, agentSlug).catch(() => null),
  ]);

  return { agent, template };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const agent = data?.agent;
  if (!agent) {
    return [{ title: "Install Agent | Astro" }];
  }
  return [
    { title: `Install ${agent.account}/${agent.name} | Astro` },
  ];
};

// --- Page ---

export default function InstallAgent({ loaderData }: Route.ComponentProps) {
  const { account, agentSlug } = useParams<{ account: string; agentSlug: string }>();
  const navigate = useNavigate();

  const { data: agent, isError } = useAgent(account ?? "", agentSlug ?? "", {
    initialData: loaderData?.agent ?? undefined,
  });

  const form = useDeployForm(account ?? "", agentSlug ?? "", {
    initialTemplate: loaderData?.template ?? undefined,
  });

  if (isError || !agent) {
    return (
      <div className="flex flex-col flex-1 bg-white">
        <div className="flex flex-col items-center justify-center py-16 px-6">
          <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
          <p className="text-stone-500 text-sm mb-4">
            The agent you're looking for doesn't exist or has been removed.
          </p>
          <Button asChild>
            <Link to="/hire">Browse Agents</Link>
          </Button>
        </div>
      </div>
    );
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.trySubmit()) return;
    try {
      await form.deploy();
      navigate("/agents");
    } catch {
      // Error is captured in form.deployError
    }
  };

  return (
    <ProtectedRoute>
      <div className="flex flex-col flex-1 bg-white">
        <PageBreadcrumb
          items={[
            { label: "Browse Agents", to: "/hire" },
            {
              label: (
                <>
                  {agent.account} <span className="text-stone-400">/</span> {agent.name}
                </>
              ),
              to: `/${agent.account}/${agent.name}`,
            },
            { label: "Install" },
          ]}
        />

        <div className="flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="w-full max-w-3xl mx-auto px-6 pt-8 pb-12 md:px-8 md:pt-10">
          {/* Header */}
          <div className="flex items-start gap-4 mb-8">
            <AgentIdentity account={agent.account} name={agent.name} size={48} className="size-12 shrink-0 rounded-lg overflow-hidden" />
            <div>
              <h1 className="text-2xl font-semibold mb-1">
                Install <span className="text-muted-foreground font-normal">{agent.account}/</span>{agent.name}
              </h1>
              <p className="text-sm text-muted-foreground">
                Configure and launch this agent
              </p>
            </div>
          </div>

          {form.templateErrorMessage && (
            <div className="mb-8">
              <ErrorPanel>{form.templateErrorMessage}</ErrorPanel>
            </div>
          )}

          {form.template && (
            <div className="space-y-8">
              {/* Interfaces */}
              <FormSection title="Messaging" description="Choose how you want to interact with the agent.">
                <InterfacesPicker
                  selected={form.selectedAdapters}
                  onChange={form.setSelectedAdapters}
                  adapterCredDefs={form.allAdapterCredDefs}
                  adapterCredentials={form.adapterCredentials}
                  onAdapterCredentialsChange={form.setAdapterCredentials}
                  showError={!!form.errors.adapters}
                  adapterErrorKeys={form.errors.adapterCredentials}
                />
              </FormSection>

              {/* Required variables */}
              {form.requiredVariables.length > 0 && (
                <FormSection title="Configuration" description="Required configuration for this agent.">
                  <VariableFields
                    variables={form.requiredVariables}
                    values={form.variableValues}
                    onChange={form.setVariableValues}
                    errorKeys={form.errors.credentials}
                  />
                </FormSection>
              )}

              {/* Optional variables */}
              {form.optionalVariables.length > 0 && (
                <FormSection title="Optional credentials" description="These are not required but enable additional functionality.">
                  <VariableFields
                    variables={form.optionalVariables}
                    values={form.variableValues}
                    onChange={form.setVariableValues}
                  />
                </FormSection>
              )}

              {/* Error */}
              {form.deployError && (
                <ErrorPanel title="Deployment failed">{form.deployError}</ErrorPanel>
              )}

              {/* Deploy button */}
              <hr className="border-border" />
              <div className="flex justify-end gap-3">
                <Button
                  type="button"
                  variant="ghost"
                  size="lg"
                  asChild
                >
                  <Link to={`/${agent.account}/${agent.name}`}>Cancel</Link>
                </Button>
                <Button
                  type="submit"
                  size="lg"
                  disabled={form.isDeploying}
                >
                  {form.isDeploying ? (
                    <>
                      <Loader2 size={16} className="animate-spin" />
                      Deploying...
                    </>
                  ) : (
                    <>
                      <Rocket size={16} />
                      Launch Agent
                    </>
                  )}
                </Button>
              </div>
            </div>
          )}
        </form>
        </div>
      </div>
    </ProtectedRoute>
  );
}
