import { useEffect, useState } from "react";
import { Loader2, Rocket, Play, Check } from "lucide-react";
import { Cog6ToothIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { useTriggerIngestion, useUploadDeploymentAvatar } from "@/api/queries/deployments";
import { useDeployForm, slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { Tag } from "@/components/Tag";
import type { AgentDeployment } from "@/lib/api";
import { bustDeploymentAvatar, useDeploymentAvatarBust } from "@/lib/avatar-bust";

const PANEL_FORM_ID = "configure-side-panel-form";
const PANEL_SHELL_CLASS = "flex h-full w-full flex-col";
const PANEL_HEADER_CLASS = "flex h-[63px] shrink-0 items-center gap-2 border-b border-border px-5";

interface ConfigurePanelProps {
  deployment: AgentDeployment;
  account: string;
  onClose: () => void;
  onRedeployStart?: () => void;
  onRedeploy?: () => void;
  fullPage?: boolean;
  revisionOverride?: number;
  readOnly?: boolean;
  isNewBuild?: boolean;
  newBuildId?: string;
  rollbackContext?: { revision: number; buildId: string };
}

function ConfigurePanelContent({ deployment, account, onClose, onRedeployStart, onRedeploy, fullPage = false, readOnly = false, revisionNumber, isNewBuild, newBuildId, rollbackContext }: {
  deployment: AgentDeployment;
  account: string;
  onClose: () => void;
  onRedeployStart?: () => void;
  onRedeploy?: () => void;
  fullPage?: boolean;
  readOnly?: boolean;
  revisionNumber?: number;
  isNewBuild?: boolean;
  newBuildId?: string;
  rollbackContext?: { revision: number; buildId: string };
}) {
  const form = useDeployForm(account, deployment.name, {
    deploymentId: deployment.id,
    build: newBuildId,
    revision: revisionNumber,
  });

  const uploadDeploymentAvatar = useUploadDeploymentAvatar(account);
  const deploymentAvatarBust = useDeploymentAvatarBust(deployment.id);

  const shellClass = fullPage
    ? "flex min-h-full w-full flex-col bg-surface"
    : PANEL_SHELL_CLASS;

  // Show loading/error before template is ready.
  if (form.templateLoading || !form.template) {
    return (
      <div className={shellClass}>
        <div className={PANEL_HEADER_CLASS}>
          <Cog6ToothIcon className="size-3.5 text-primary shrink-0" />
          <span className="flex-1 text-heading-4 font-semibold text-foreground">Configure</span>
          <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
            <XMarkIcon className="size-4" />
          </Button>
        </div>
        <div className="flex flex-1 items-center justify-center px-5">
          {form.templateErrorMessage
            ? <p className="text-body-sm text-destructive">{form.templateErrorMessage}</p>
            : <Loader2 className="size-5 animate-spin text-muted-foreground" />
          }
        </div>
      </div>
    );
  }

  const template = form.template;

  const handleSubmit = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!form.trySubmit()) return;
    onRedeployStart?.();
    try {
      const result = await form.deploy();
      if (!result) return; // Validation failed — error is shown in form.deployError
      onClose();
      onRedeploy?.();
    } catch {
      // captured in form.deployError
    }
  };

  const manualIngestions = deployment.manual_ingestions ?? [];
  const formClass = fullPage
    ? "flex min-h-0 flex-1 flex-col"
    : "dp-scroll flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain";

  return (
    <div className={shellClass}>
      <div className={PANEL_HEADER_CLASS}>
        <Cog6ToothIcon className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 min-w-0 text-heading-4 font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <XMarkIcon className="size-4" />
        </Button>
      </div>

      {/* Context banner — shown when panel has a special mode */}
      {(rollbackContext || readOnly || isNewBuild) && (
        <div
          className="relative flex items-center gap-2 px-5 py-[13.5px] shrink-0 border-b border-border"
          style={{
            background: rollbackContext
              ? "color-mix(in oklch, var(--color-amber-600) 8%, transparent)"
              : isNewBuild
              ? "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)"
              : "var(--muted)",
          }}
        >
          {/* Left accent bar — absolutely positioned so it doesn't shift content */}
          <div
            className="absolute left-0 inset-y-0 w-0.5"
            style={{
              background: rollbackContext
                ? "var(--color-amber-600)"
                : isNewBuild
                ? "var(--color-yellow-700)"
                : "var(--border-strong)",
            }}
          />
          {rollbackContext && (
            <>
              <span className="font-mono text-mono-sm text-amber-700 font-medium">Rollback</span>
              <Tag className="px-1.5 h-[18px]">Config {rollbackContext.revision}</Tag>
              <Tag className="px-1.5 h-[18px]">{rollbackContext.buildId.slice(0, 8)}</Tag>
            </>
          )}
          {readOnly && !rollbackContext && (
            <>
              <span className="font-mono text-mono-sm text-muted-foreground font-medium">Viewing</span>
              {revisionNumber != null && (
                <Tag className="px-1.5 h-[18px]">Config {revisionNumber}</Tag>
              )}
              <span className="font-mono text-mono-sm text-faint-foreground">(read only)</span>
            </>
          )}
          {isNewBuild && !rollbackContext && (
            <>
              <span className="font-mono text-mono-sm font-medium" style={{ color: "var(--color-yellow-700)" }}>Update</span>
              <Tag color="yellow" className="px-1.5 h-[18px]">{deployment.build_id.slice(0, 8)}</Tag>
              <span className="font-mono text-[11px] text-muted-foreground">→</span>
              <Tag color="yellow" className="px-1.5 h-[18px]">{template.source.build.slice(0, 8)}</Tag>
            </>
          )}
        </div>
      )}

      <form id={PANEL_FORM_ID} onSubmit={readOnly ? (e) => e.preventDefault() : handleSubmit} className={formClass}>
        <div className={readOnly ? "pointer-events-none select-text flex flex-col min-h-0 flex-1" : "flex flex-col min-h-0 flex-1"}>
          <div className="px-6 pt-5 pb-6">
            <DeployFormFields
              form={form}
              hideAccountPicker
              avatar={{
                url: deploymentAvatarBust ?? deployment.avatar_url,
                account,
                blueprintName: deployment.name,
                onUpload: readOnly ? undefined : async (file) => {
                  await uploadDeploymentAvatar.mutateAsync({ id: deployment.id, file });
                  bustDeploymentAvatar(deployment.id, file);
                },
                isPending: uploadDeploymentAvatar.isPending,
              }}
              ingestionExtra={
                !readOnly && manualIngestions.length > 0 ? (
                  <ManualTriggers
                    deploymentId={deployment.id}
                    names={manualIngestions}
                    account={account}
                    hasBorderTop={form.scheduleIngestions.length > 0}
                  />
                ) : undefined
              }
            />
          </div>
        </div>
      </form>

      <div className="shrink-0 border-t border-border bg-surface/95 px-5 py-4 backdrop-blur supports-[backdrop-filter]:bg-surface/90">
        {readOnly ? (
          <Button type="button" variant="outline" className="w-full" onClick={onClose}>
            Close
          </Button>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="flex gap-2">
              <Button type="button" variant="outline" className="flex-1" onClick={onClose} disabled={form.isDeploying}>
                Discard
              </Button>
              <Button type="submit" form={PANEL_FORM_ID} variant="default" disabled={form.isDeploying} className="flex-1">
                {form.isDeploying ? <Loader2 className="size-3.5 animate-spin" /> : <Rocket className="size-3.5" />}
                {form.isDeploying ? "Redeploying…" : "Redeploy"}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function ManualTriggers({
  deploymentId,
  names,
  account,
  hasBorderTop,
}: {
  deploymentId: string;
  names: string[];
  account: string;
  hasBorderTop: boolean;
}) {
  const triggerMutation = useTriggerIngestion(account);
  const [triggeredName, setTriggeredName] = useState<string | null>(null);

  useEffect(() => {
    if (!triggeredName) return;
    const timer = setTimeout(() => setTriggeredName(null), 2000);
    return () => clearTimeout(timer);
  }, [triggeredName]);

  return (
    <div className={hasBorderTop ? "mt-6 pt-6 border-t border-border" : ""}>
      <p className="mb-3 text-body font-medium text-foreground">Manual Triggers</p>
      <div className="flex flex-wrap gap-2">
        {names.map((name) => {
          const isTriggering = triggerMutation.isPending && triggerMutation.variables?.ingestion === name;
          const justTriggered = triggeredName === name;
          return (
            <Button
              key={name}
              type="button"
              variant="outline"
              size="sm"
              disabled={isTriggering || justTriggered}
              onClick={() =>
                triggerMutation.mutate(
                  { deploymentId, ingestion: name },
                  { onSuccess: () => setTriggeredName(name) },
                )
              }
            >
              {justTriggered ? (
                <Check className="size-3.5 text-primary" />
              ) : isTriggering ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Play className="size-3.5" />
              )}
              {slugToTitle(name)}
            </Button>
          );
        })}
      </div>
      {triggerMutation.isError && (
        <p className="text-sm text-destructive mt-2">Failed to trigger ingestion. Please try again.</p>
      )}
    </div>
  );
}

export function ConfigurePanel({ deployment, account, onClose, onRedeployStart, onRedeploy, fullPage = false, revisionOverride, readOnly = false, isNewBuild, newBuildId, rollbackContext }: ConfigurePanelProps) {
  const revision = revisionOverride ?? rollbackContext?.revision;
  return (
    <ConfigurePanelContent
      key={revision ?? 'current'}
      deployment={deployment}
      account={account}
      onClose={onClose}
      onRedeployStart={onRedeployStart}
      onRedeploy={onRedeploy}
      fullPage={fullPage}
      readOnly={readOnly}
      revisionNumber={revision}
      isNewBuild={isNewBuild}
      newBuildId={newBuildId}
      rollbackContext={rollbackContext}
    />
  );
}
