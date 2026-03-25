import { useState, useRef, useCallback } from "react";
import { useParams, Link, useNavigate } from "react-router";
import type { Route } from "./+types/DeployBlueprint";
import { ArrowLeft, Loader2, Rocket } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useBlueprint } from "@/api/queries/blueprints";
import { useUploadDeploymentAvatar } from "@/api/queries/deployments";
import { createServerApi } from "@/lib/api.server";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { useAuth } from "@/lib/auth";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";

// --- Loader & Meta ---

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";

  if (!account || !agentSlug) {
    return { agent: null, template: null };
  }

  const [agent, template] = await Promise.all([
    api.getBlueprint(account, agentSlug).catch(() => null),
    api.getDeploymentTemplate(account, agentSlug).catch(() => null),
  ]);

  return { agent, template };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const agent = data?.agent;
  if (!agent) {
    return [{ title: "Deploy Agent | Astro" }];
  }
  return [
    { title: `Deploy ${agent.account}/${agent.name} | Astro` },
  ];
};

// --- Page ---

export default function DeployBlueprint({ loaderData }: Route.ComponentProps) {
  const { account, agentSlug } = useParams<{ account: string; agentSlug: string }>();
  const navigate = useNavigate();

  const { data: agent, isError } = useBlueprint(account ?? "", agentSlug ?? "", {
    initialData: loaderData?.agent ?? undefined,
  });

  const form = useDeployForm(account ?? "", agentSlug ?? "", {
    initialTemplate: loaderData?.template ?? undefined,
  });

  const { personalAccount } = useAuth();
  const uploadDeploymentAvatar = useUploadDeploymentAvatar(personalAccount?.name ?? "");

  // Staged avatar blob — held in memory until deploy succeeds
  const [stagedPreviewUrl, setStagedPreviewUrl] = useState<string | null>(null);
  const stagedBlobRef = useRef<Blob | null>(null);

  const handleStageAvatar = useCallback((blob: Blob | null) => {
    if (stagedPreviewUrl) URL.revokeObjectURL(stagedPreviewUrl);
    if (blob) {
      const url = URL.createObjectURL(blob);
      setStagedPreviewUrl(url);
      stagedBlobRef.current = blob;
    } else {
      setStagedPreviewUrl(null);
      stagedBlobRef.current = null;
    }
  }, [stagedPreviewUrl]);

  if (isError || !agent) {
    return (
      <div className="flex flex-col flex-1 bg-surface">
        <div className="flex flex-col items-center justify-center py-16 px-6">
          <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
          <p className="text-stone-500 text-sm mb-4">
            The agent you're looking for doesn't exist or has been removed.
          </p>
          <Button asChild>
            <Link to="/blueprints">Blueprints</Link>
          </Button>
        </div>
      </div>
    );
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.trySubmit()) return;
    try {
      const result = await form.deploy();
      // Upload staged avatar to the newly created deployment
      if (result?.deployment_id && stagedBlobRef.current) {
        try {
          await uploadDeploymentAvatar.mutateAsync({
            id: result.deployment_id,
            file: stagedBlobRef.current,
          });
        } catch {
          // Avatar upload failure shouldn't block navigation — deployment succeeded
        }
      }
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
              className="flex items-center justify-center p-1 text-faint-foreground hover:text-foreground transition-colors"
            >
              <ArrowLeft className="size-4" />
            </Link>
            <BlueprintIdentity
              account={agent.account}
              name={agent.name}
              size={32}
              avatarUrl={agent.avatar_url}
              className="size-8 shrink-0 rounded-sm overflow-hidden"
            />
            <div>
              <h1 className="text-sm font-bold text-foreground">
                Deploy {agent.account}/{agent.name}
              </h1>
              <div className="text-body-sm text-faint-foreground">
                Configure and deploy this agent to your account
              </div>
            </div>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="w-full max-w-xl mx-auto px-6 pt-10 pb-20 md:px-8">
          <DeployFormFields
            form={form}
            avatar={{
              url: agent.avatar_url,
              account: agent.account,
              blueprintName: agent.name,
              onStage: handleStageAvatar,
              stagedPreviewUrl: stagedPreviewUrl ?? undefined,
            }}
          />

          {form.template && (
            <>
              {/* Deploy button */}
              <hr className="border-border mt-12" />
              <div className="flex justify-end gap-3 mt-12">
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
                      Deploy
                    </>
                  )}
                </Button>
              </div>
            </>
          )}
        </form>
        </div>
      </div>
    </ProtectedRoute>
  );
}
