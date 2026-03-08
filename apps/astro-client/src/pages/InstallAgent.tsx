import { useParams, Link, useNavigate } from "react-router";
import type { Route } from "./+types/InstallAgent";
import { ArrowLeft, Loader2, Rocket } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAgent } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { AccountPicker } from "@/components/deploy/AccountPicker";
import { InterfacesPicker } from "@/components/deploy/InterfacesPicker";
import { VariableFields } from "@/components/deploy/VariableFields";
import { FormSection } from "@/components/deploy/FormSection";
import { ErrorPanel } from "@/components/deploy/ErrorPanel";
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
      <div className="flex flex-col flex-1 bg-surface">
        <div className="flex flex-col items-center justify-center py-16 px-6">
          <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
          <p className="text-stone-500 text-sm mb-4">
            The agent you're looking for doesn't exist or has been removed.
          </p>
          <Button asChild>
            <Link to="/browse">Browse Agents</Link>
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
      <div className="flex flex-col flex-1 bg-surface">
        <header className="sticky top-0 z-10 flex items-center justify-between px-6 min-h-[52px] bg-stone-200 border-b border-stone-300 dark:bg-background dark:border-border">
          <div className="flex items-center gap-3">
            <Link
              to={`/${agent.account}/${agent.name}`}
              className="flex items-center justify-center p-1 text-ink-faint hover:text-foreground transition-colors"
            >
              <ArrowLeft className="size-4" />
            </Link>
            <AgentIdentity
              account={agent.account}
              name={agent.name}
              size={32}
              className="size-8 shrink-0 rounded-sm overflow-hidden"
            />
            <div>
              <div className="text-sm font-bold text-primary">
                Install {agent.name}
              </div>
              <div className="font-mono text-[10px] text-ink-faint">
                {agent.account}/{agent.name}
              </div>
            </div>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="w-full max-w-xl mx-auto px-6 pt-10 pb-20 md:px-8">
          {form.templateErrorMessage && (
            <div className="mb-8">
              <ErrorPanel>{form.templateErrorMessage}</ErrorPanel>
            </div>
          )}

          {form.template && (
            <div className="space-y-12">
              {/* Agent name & account */}
              <FormSection title="General" description="Choose what to call your agent and where to install it.">
                <div className="space-y-5">
                  <div>
                    <label className="text-[13px] font-semibold text-foreground mb-1 block">Agent Name</label>
                    <Input
                      value={form.deployName}
                      onChange={(e) => form.setDeployName(e.target.value)}
                      placeholder="My Agent"
                      maxLength={64}
                      aria-invalid={!!form.errors.deployName}
                    />
                    {form.errors.deployName && (
                      <p className="text-sm text-destructive mt-1">{form.errors.deployName}</p>
                    )}
                  </div>

                  {form.accounts.length > 1 && (
                    <div>
                      <label className="text-[13px] font-semibold text-foreground mb-1 block">Install to</label>
                      <AccountPicker
                        accounts={form.accounts}
                        selected={form.targetAccount}
                        onChange={form.setTargetAccount}
                      />
                    </div>
                  )}
                </div>
              </FormSection>

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
                  size="default"
                  asChild
                >
                  <Link to={`/${agent.account}/${agent.name}`}>Cancel</Link>
                </Button>
                <Button
                  type="submit"
                  size="default"
                  disabled={form.isDeploying}
                  className="px-6 has-[>svg]:px-6"
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
