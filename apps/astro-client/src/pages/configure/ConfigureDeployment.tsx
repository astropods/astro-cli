import { useEffect, useMemo, useState } from "react";
import { useNavigate, useOutletContext } from "react-router";
import { Play, Check, Loader2 } from "lucide-react";
import type { ConfigureContext } from "./types";
import { deploymentPath } from "@/lib/routes";
import { Button } from "@/components/ui/button";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { DeployFormActionBar } from "@/components/deploy/DeployFormActionBar";
import { extractInitialValues } from "@/components/deploy/extractInitialValues";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";
import { useTriggerIngestion } from "@/api/queries/deployments";

const FORM_ID = "configure-deployment-form";

export default function ConfigureDeployment() {
  const {
    account,
    deployment,
    template,
    hasNewerBuildAvailable,
    currentBuildId,
    latestBuildId,
  } = useOutletContext<ConfigureContext>();
  const navigate = useNavigate();

  const basePath = deploymentPath(account, deployment.id);

  const initialValues = useMemo(
    () => extractInitialValues(template, account),
    [template, account],
  );

  const form = useDeployForm(account, deployment.name, {
    initialTemplate: template,
    skipTemplateFetch: true,
    initialValues,
  });

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

  const deployError = form.deployError;
  useEffect(() => {
    if (!deployError) return;
    const doc = document.documentElement;
    const distanceFromBottom = doc.scrollHeight - window.scrollY - window.innerHeight;
    if (distanceFromBottom > 100) {
      window.scrollTo({ top: doc.scrollHeight, behavior: "smooth" });
    }
  }, [deployError]);

  const manualIngestions = deployment.manual_ingestions ?? [];
  const triggerMutation = useTriggerIngestion(account);
  const [triggeredName, setTriggeredName] = useState<string | null>(null);

  useEffect(() => {
    if (!triggeredName) return;
    const timer = setTimeout(() => setTriggeredName(null), 2000);
    return () => clearTimeout(timer);
  }, [triggeredName]);

  const handleTrigger = (name: string) => {
    triggerMutation.mutate(
      { deploymentId: deployment.id, ingestion: name },
      { onSuccess: () => setTriggeredName(name) },
    );
  };

  const ingestionExtra = manualIngestions.length > 0 ? (
    <div className={form.scheduleIngestions.length > 0 ? "mt-6 pt-6 border-t border-border" : ""}>
      <p className="text-sm font-medium text-foreground mb-3">Manual Triggers</p>
      <div className="flex flex-wrap gap-2">
        {manualIngestions.map((name) => {
          const isTriggering = triggerMutation.isPending && triggerMutation.variables?.ingestion === name;
          const justTriggered = triggeredName === name;

          return (
            <Button
              key={name}
              type="button"
              variant="outline"
              size="sm"
              disabled={isTriggering || justTriggered}
              onClick={() => handleTrigger(name)}
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
        <p className="text-sm text-destructive mt-2">
          Failed to trigger ingestion. Please try again.
        </p>
      )}
    </div>
  ) : null;

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
    <>
      <form
        id={FORM_ID}
        onSubmit={handleSubmit}
        className={changes.isDirty || hasNewerBuildAvailable ? "pb-24" : ""}
      >
        <DeployFormFields form={form} hideAccountPicker ingestionExtra={ingestionExtra} />
      </form>

      <DeployFormActionBar
        isDirty={changes.isDirty}
        changeCount={changes.changeCount}
        requiresRedeploy={changes.requiresRedeploy || hasNewerBuildAvailable}
        showBuildUpgradeRedeploy={hasNewerBuildAvailable}
        currentBuildId={currentBuildId}
        latestBuildId={latestBuildId}
        isSaving={form.isDeploying}
        formId={FORM_ID}
        onReset={() => form.reset(initialValues)}
      />
    </>
  );
}
