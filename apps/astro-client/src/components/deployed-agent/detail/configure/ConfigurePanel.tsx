import { useEffect, useMemo, useState } from "react";
import { X, Loader2, Rocket, Play, Check } from "lucide-react";
import { Cog6ToothIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { usePrefilledDeploymentTemplate } from "@/api/queries/blueprints";
import { useTriggerIngestion, useUploadDeploymentAvatar } from "@/api/queries/deployments";
import { useDeployForm, slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { extractInitialValues } from "@/components/deploy/extractInitialValues";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";
import type { AgentDeployment } from "@/lib/api";

const PANEL_FORM_ID = "configure-side-panel-form";
const PANEL_SHELL_CLASS = "flex h-full w-[420px] flex-col border-l border-border bg-surface dark:bg-background";
const PANEL_HEADER_CLASS = "flex h-[63px] shrink-0 items-center gap-2 border-b border-border px-5";
const REDEPLOY_BUTTON_CLASS =
  "bg-[var(--color-teal-600)] text-white hover:bg-[var(--color-teal-700)] active:bg-[var(--color-teal-800)] dark:bg-[var(--color-teal-600)] dark:hover:bg-[var(--color-teal-500)] dark:active:bg-[var(--color-teal-400)]";

interface ConfigurePanelProps {
  deployment: AgentDeployment;
  account: string;
  onClose: () => void;
  onRedeployStart?: () => void;
  onRedeploy?: () => void;
  fullPage?: boolean;
}

function ConfigurePanelLoaded({ deployment, account, template, onClose, onRedeployStart, onRedeploy, fullPage = false }: {
  deployment: AgentDeployment;
  account: string;
  template: import("@/lib/api").DeploymentTemplate;
  onClose: () => void;
  onRedeployStart?: () => void;
  onRedeploy?: () => void;
  fullPage?: boolean;
}) {
  const initialValues = useMemo(() => extractInitialValues(template, account), [template, account]);
  const form = useDeployForm(account, deployment.name, { initialTemplate: template, skipTemplateFetch: true, initialValues });
  const uploadDeploymentAvatar = useUploadDeploymentAvatar(account);

  const trackedState: TrackedFormState = {
    deployName: form.deployName,
    variableValues: form.variableValues,
    selectedAdapters: form.selectedAdapters,
    adapterCredentials: form.adapterCredentials,
    ingestionSchedules: form.ingestionSchedules,
  };
  const initialTrackedState: TrackedFormState = {
    deployName: initialValues.deployName ?? "",
    variableValues: initialValues.variableValues ?? {},
    selectedAdapters: initialValues.selectedAdapters ?? ["web"],
    adapterCredentials: initialValues.adapterCredentials ?? {},
    ingestionSchedules: initialValues.ingestionSchedules ?? {},
  };
  const changes = useChangeTracking(initialTrackedState, trackedState);

  const handleSubmit = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!form.trySubmit()) return;
    onRedeployStart?.();
    try {
      await form.deploy();
      onClose();
      onRedeploy?.();
    } catch {
      // captured in form.deployError
    }
  };

  const manualIngestions = deployment.manual_ingestions ?? [];
  const shellClass = fullPage
    ? "flex min-h-full w-full flex-col bg-surface dark:bg-background"
    : PANEL_SHELL_CLASS;
  const formClass = fullPage
    ? "flex min-h-0 flex-1 flex-col"
    : "dp-scroll flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain";

  return (
    <div className={shellClass}>
      <div className={PANEL_HEADER_CLASS}>
        <Cog6ToothIcon className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-heading-4 font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>

      <form id={PANEL_FORM_ID} onSubmit={handleSubmit} className={formClass}>
        <div className="flex-1 px-6 py-5">
          <DeployFormFields
            form={form}
            hideAccountPicker
            avatar={{
              url: deployment.avatar_url,
              account,
              blueprintName: deployment.name,
              onUpload: async (file) => {
                await uploadDeploymentAvatar.mutateAsync({ id: deployment.id, file });
              },
              isPending: uploadDeploymentAvatar.isPending,
            }}
            ingestionExtra={
              manualIngestions.length > 0 ? (
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
      </form>

      <div className="sticky bottom-0 z-10 shrink-0 border-t border-border bg-surface/95 px-5 py-4 backdrop-blur supports-[backdrop-filter]:bg-surface/90">
        <div className="flex flex-col gap-2">
          <div className="flex gap-2">
            <Button type="button" variant="outline" className="flex-1" onClick={onClose} disabled={form.isDeploying}>
              Discard
            </Button>
            <Button type="submit" form={PANEL_FORM_ID} disabled={form.isDeploying} className={`flex-1 ${REDEPLOY_BUTTON_CLASS}`}>
              {form.isDeploying ? <Loader2 className="size-3.5 animate-spin" /> : <Rocket className="size-3.5" />}
              {form.isDeploying ? "Redeploying…" : changes.requiresRedeploy ? "Save & Redeploy" : "Redeploy"}
            </Button>
          </div>
          {changes.isDirty && (
            <Button type="button" variant="ghost" className="w-full" onClick={() => form.reset(initialValues)}>
              Reset changes
            </Button>
          )}
        </div>
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

export function ConfigurePanel({ deployment, account, onClose, onRedeployStart, onRedeploy, fullPage = false }: ConfigurePanelProps) {
  const { data: template, isLoading, isError } = usePrefilledDeploymentTemplate(account, deployment.name, deployment.id);
  const shellClass = fullPage
    ? "flex min-h-full w-full flex-col bg-surface dark:bg-background"
    : PANEL_SHELL_CLASS;

  const shell = (children: React.ReactNode) => (
    <div className={shellClass}>
      <div className={PANEL_HEADER_CLASS}>
        <Cog6ToothIcon className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-heading-4 font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>
      <div className="flex flex-1 items-center justify-center px-5">{children}</div>
    </div>
  );

  if (isLoading) return shell(<Loader2 className="size-5 animate-spin text-muted-foreground" />);
  if (isError || !template) return shell(<p className="text-body-sm text-destructive">Failed to load configuration.</p>);
  return (
    <ConfigurePanelLoaded
      deployment={deployment}
      account={account}
      template={template}
      onClose={onClose}
      onRedeployStart={onRedeployStart}
      onRedeploy={onRedeploy}
      fullPage={fullPage}
    />
  );
}
