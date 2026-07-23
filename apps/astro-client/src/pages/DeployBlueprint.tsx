import { useState, useRef, useCallback } from "react";
import { useParams, Link, useNavigate, useSearchParams } from "react-router";
import type { Route } from "./+types/DeployBlueprint";
import { ArrowLeft, Loader2, Rocket } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useBlueprint } from "@/api/queries/blueprints";
import { useUploadDeploymentAvatar } from "@/api/queries/deployments";
import { createServerApi } from "@/lib/api.server";
import type { AvatarColors } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { BlueprintVersionPicker } from "@/components/deploy/BlueprintVersionPicker";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { accountBlueprintsPath, dashboardPath } from "@/lib/routes";

// --- Loader & Meta ---

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";

  if (!account || !agentSlug) {
    return { agent: null, template: null };
  }

  const [agent, templateResponse] = await Promise.all([
    api.getBlueprint(account, agentSlug).catch(() => null),
    api.postDeploymentTemplate(account, agentSlug).catch(() => null),
  ]);

  return { agent, templateResponse };
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
  const [searchParams, setSearchParams] = useSearchParams();

  const { data: agent, isError } = useBlueprint(account ?? "", agentSlug ?? "", {
    initialData: loaderData?.agent ?? undefined,
  });

  const latestBuildId = agent?.versions[0]?.build_id;
  const selectedBuildId = searchParams.get("build") ?? latestBuildId;
  const selectedHistoricalBuild = !!selectedBuildId && selectedBuildId !== latestBuildId;

  const form = useDeployForm(account ?? "", agentSlug ?? "", {
    initialTemplateResponse:
      !selectedHistoricalBuild &&
      loaderData?.templateResponse &&
      loaderData.templateResponse.template.source.build === selectedBuildId
        ? loaderData.templateResponse
        : undefined,
    build: selectedBuildId,
    allowedTargetAccounts: agent?.visibility === "private" && account ? [account] : undefined,
  });
  const hasTemplateSwitchError = !!form.template && !!form.templateError;

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
      <div className="flex flex-col flex-1 bg-background">
        <div className="flex flex-col items-center justify-center py-16 px-6">
          <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
          <p className="text-stone-500 text-sm mb-4">
            The agent you're looking for doesn't exist or has been removed.
          </p>
          <Button asChild>
            <Link to={accountBlueprintsPath}>Blueprints</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (agent.versions.length === 0) {
    return (
      <div className="flex flex-col flex-1 bg-background">
        <div className="flex flex-col items-center justify-center py-16 px-6">
          <h1 className="text-xl font-semibold mb-3">Blueprint not ready</h1>
          <p className="text-stone-500 text-sm mb-4">
            Push your blueprint with <code className="font-mono bg-muted px-1 rounded">ast push</code> before deploying.
          </p>
          <Button asChild>
            <Link to={`/${account}/${agentSlug}`}>Continue setup</Link>
          </Button>
        </div>
      </div>
    );
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (hasTemplateSwitchError) return;
    if (!form.trySubmit()) return;
    try {
      const result = await form.deploy();
      if (!result) return; // Validation failed — error is shown in form.deployError

      // Upload staged avatar before navigating so we can pass the server URL
      // (not the local blob URL) in nav state — prevents a blob→server URL
      // transition in the reveal overlay that causes an avatar flicker.
      // The upload response also includes extracted avatar_colors.
      let revealAvatarUrl: string | null | undefined = stagedPreviewUrl;
      let revealAvatarColors: AvatarColors | undefined = agent.avatar_colors;
      if (result.deployment_id && stagedBlobRef.current) {
        try {
          const avatarResult = await uploadDeploymentAvatar.mutateAsync({
            id: result.deployment_id,
            file: stagedBlobRef.current,
          });
          revealAvatarUrl = avatarResult.avatar_url;
          if (avatarResult.avatar_colors) {
            revealAvatarColors = avatarResult.avatar_colors;
          }
        } catch {
          // Avatar upload failure shouldn't block navigation — deployment succeeded
        }
      }
      const destination = `${dashboardPath}?account=${encodeURIComponent(form.targetAccount)}`;

      navigate(destination, {
        state: {
          revealDeploymentId: result.deployment_id,
          revealAgentName: agent.name,
          revealDisplayName: form.deployName,
          revealAvatarUrl,
          revealAvatarColors,
        },
      });
    } catch {
      // Error is captured in form.deployError
    }
  };

  return (
      <div className="flex flex-col flex-1 bg-background">
        <header className="sticky top-0 z-10 flex items-center justify-between px-6 min-h-[52px] bg-background border-b border-border">
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
              url={agent.avatar_url}
              size={32}
              className="size-8 shrink-0 rounded-sm overflow-hidden"
            />
            <div>
              <h1 className="text-sm font-semibold text-foreground">
                Deploy {agent.account}/{agent.name}
              </h1>
              <div className="text-body-sm text-muted-foreground">
                Configure and deploy this agent to your account
              </div>
            </div>
          </div>
        </header>

        <div className="flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="w-full max-w-xl mx-auto px-6 pt-10 pb-20 md:px-8">
          {selectedBuildId && (
            <BlueprintVersionPicker
              versions={agent.versions}
              selectedBuildId={selectedBuildId}
              latestBuildId={latestBuildId}
              onBuildChange={(buildId) => {
                setSearchParams(
                  buildId === latestBuildId ? {} : { build: buildId },
                  { replace: true },
                );
              }}
              loading={form.templateLoading}
              error={hasTemplateSwitchError ? form.templateError : undefined}
              recovery={selectedHistoricalBuild ? {
                label: "Use latest build",
                onClick: () => setSearchParams({}, { replace: true }),
              } : undefined}
            />
          )}
          <fieldset
            disabled={form.templateLoading || hasTemplateSwitchError}
            aria-busy={form.templateLoading || hasTemplateSwitchError}
            className={
              form.templateLoading || hasTemplateSwitchError
                ? "min-w-0 border-0 p-0 pointer-events-none opacity-55 transition-opacity duration-200"
                : "min-w-0 border-0 p-0 opacity-100 transition-opacity duration-200"
            }
          >
            <DeployFormFields
              form={form}
              hideTemplateError={hasTemplateSwitchError}
              avatar={{
                url: agent.avatar_url,
                account: agent.account,
                blueprintName: agent.name,
                onStage: handleStageAvatar,
                stagedPreviewUrl: stagedPreviewUrl ?? undefined,
              }}
            />
          </fieldset>

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
                  disabled={form.isDeploying || form.templateLoading}
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
  );
}
