import { useEffect, useMemo, useState } from "react";
import { Settings2, X, Loader2, Rocket, Play, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import { usePrefilledDeploymentTemplate } from "@/api/queries/agents";
import { useTriggerIngestion } from "@/api/queries/deployments";
import { useDeployForm, slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { extractInitialValues } from "@/components/deploy/extractInitialValues";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";
import type { AgentDeployment } from "@/lib/api";

const PANEL_FORM_ID = "configure-side-panel-form";

interface ConfigurePanelProps {
  deployment: AgentDeployment;
  account: string;
  onClose: () => void;
  onRedeploy?: () => void;
}

function ConfigurePanelLoaded({ deployment, account, template, onClose, onRedeploy }: {
  deployment: AgentDeployment;
  account: string;
  template: import("@/lib/api").DeploymentTemplate;
  onClose: () => void;
  onRedeploy?: () => void;
}) {
  const initialValues = useMemo(() => extractInitialValues(template, account), [template, account]);
  const form = useDeployForm(account, deployment.name, { initialTemplate: template, skipTemplateFetch: true, initialValues });

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
    try {
      await form.deploy();
      onClose();
      onRedeploy?.();
    } catch {
      // captured in form.deployError
    }
  };

  const manualIngestions = deployment.manual_ingestions ?? [];

  return (
    <div className="flex flex-col h-full w-[420px] bg-background border-l border-border">
      <div className="flex items-center gap-2 h-[63px] shrink-0 px-4 border-b border-border">
        <Settings2 className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-sm font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>

      <form id={PANEL_FORM_ID} onSubmit={handleSubmit} className="flex-1 min-h-0 flex flex-col">
        <div className="flex-1 overflow-y-auto px-5 py-6">
          <DeployFormFields
            form={form}
            hideAccountPicker
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

        {changes.isDirty && (
          <div className="flex flex-col gap-2 p-4 border-t border-border bg-muted/40 shrink-0">
            <Button type="submit" disabled={form.isDeploying} className="w-full">
              {form.isDeploying ? <Loader2 className="size-3.5 animate-spin" /> : <Rocket className="size-3.5" />}
              {form.isDeploying ? "Redeploying…" : changes.requiresRedeploy ? "Save & Redeploy" : "Save"}
            </Button>
            <Button type="button" variant="ghost" className="w-full" onClick={() => form.reset(initialValues)}>
              Cancel
            </Button>
          </div>
        )}
      </form>
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
      <p className="text-sm font-medium text-foreground mb-3">Manual Triggers</p>
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
                <Check className="size-3.5 text-green-600" />
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

export function ConfigurePanel({ deployment, account, onClose, onRedeploy }: ConfigurePanelProps) {
  const { data: template, isLoading, isError } = usePrefilledDeploymentTemplate(account, deployment.name, deployment.id);

  const shell = (children: React.ReactNode) => (
    <div className="flex flex-col h-full w-[420px] bg-background border-l border-border">
      <div className="flex items-center gap-2 h-[63px] shrink-0 px-4 border-b border-border">
        <Settings2 className="size-3.5 text-primary shrink-0" />
        <span className="flex-1 text-sm font-semibold text-foreground">Configure</span>
        <Button variant="ghost" size="icon" className="size-7 shrink-0" onClick={onClose}>
          <X className="size-4" />
        </Button>
      </div>
      <div className="flex-1 flex items-center justify-center">{children}</div>
    </div>
  );

  if (isLoading) return shell(<Loader2 className="size-5 animate-spin text-muted-foreground" />);
  if (isError || !template) return shell(<p className="text-xs text-destructive">Failed to load configuration.</p>);
  return (
    <ConfigurePanelLoaded
      deployment={deployment}
      account={account}
      template={template}
      onClose={onClose}
      onRedeploy={onRedeploy}
    />
  );
}
