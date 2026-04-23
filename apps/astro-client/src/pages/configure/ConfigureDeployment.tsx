import { useEffect, useState, type SyntheticEvent } from "react";
import { useNavigate, useOutletContext, type MetaFunction } from "react-router";
import { Play, Check, Loader2 } from "lucide-react";
import type { ConfigureContext } from "./types";
import { deploymentPath } from "@/lib/routes";
import { Button } from "@/components/ui/button";
import { useDeployForm, slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { DeployFormActionBar } from "@/components/deploy/DeployFormActionBar";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";
import { useTriggerIngestion, useUploadDeploymentAvatar } from "@/api/queries/deployments";
import { bustDeploymentAvatar, useDeploymentAvatarBust } from "@/lib/avatar-bust";

export const meta: MetaFunction = () => [{ title: "Configure Deployment | Astro" }];

const FORM_ID = "configure-deployment-form";

export default function ConfigureDeployment() {
  const {
    account,
    deployment,
    hasNewerBuildAvailable,
    currentBuildId,
    latestBuildId,
  } = useOutletContext<ConfigureContext>();
  const navigate = useNavigate();

  const basePath = deploymentPath(account, deployment.id);

  const form = useDeployForm(account, deployment.name, {
    deploymentId: deployment.id,
  });

  const initialValues = form.initialValues;

  const uploadDeploymentAvatar = useUploadDeploymentAvatar(account);
  const deploymentAvatarBust = useDeploymentAvatarBust(deployment.id);

  const trackedState: TrackedFormState = {
    deployName: form.deployName,
    variableValues: form.variableValues,
    selectedAdapters: form.selectedAdapters,
    adapterCredentials: form.adapterCredentials,
    ingestionSchedules: form.ingestionSchedules,
  };
  const initialTrackedState: TrackedFormState = {
    deployName: initialValues?.deployName ?? "",
    variableValues: initialValues?.variableValues ?? {},
    selectedAdapters: initialValues?.selectedAdapters ?? ["web"],
    adapterCredentials: initialValues?.adapterCredentials ?? {},
    ingestionSchedules: initialValues?.ingestionSchedules ?? {},
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

  const saveAndRedeploy = async (e: SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!form.trySubmit()) return;
    try {
      const result = await form.deploy();
      if (!result) return; // Validation failed — error is shown in form.deployError
      navigate(basePath);
    } catch {
      // form.deployError captures the message
    }
  };

  return (
    <>
      <form
        id={FORM_ID}
        onSubmit={saveAndRedeploy}
        className={changes.isDirty || hasNewerBuildAvailable ? "pb-24" : ""}
      >
        <DeployFormFields
          form={form}
          hideAccountPicker
          avatar={{
            url: deploymentAvatarBust ?? deployment.avatar_url,
            account,
            blueprintName: deployment.name,
            onUpload: async (file) => {
              await uploadDeploymentAvatar.mutateAsync({ id: deployment.id, file });
              bustDeploymentAvatar(deployment.id, file);
            },
            isPending: uploadDeploymentAvatar.isPending,
          }}
          ingestionExtra={
            manualIngestions.length > 0
              ? <ManualTriggers
                  deploymentId={deployment.id}
                  names={manualIngestions}
                  account={account}
                  hasBorderTop={form.scheduleIngestions.length > 0}
                />
              : undefined
          }
        />
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
        onReset={() => form.reset(initialValues ?? undefined)}
      />
    </>
  );
}

interface ManualTriggersProps {
  deploymentId: string;
  names: string[];
  account: string;
  hasBorderTop: boolean;
}

function ManualTriggers({ deploymentId, names, account, hasBorderTop }: ManualTriggersProps) {
  const triggerMutation = useTriggerIngestion(account);
  const [triggeredName, setTriggeredName] = useState<string | null>(null);

  useEffect(() => {
    if (!triggeredName) return;
    const timer = setTimeout(() => setTriggeredName(null), 2000);
    return () => clearTimeout(timer);
  }, [triggeredName]);

  const handleTrigger = (name: string) => {
    triggerMutation.mutate(
      { deploymentId, ingestion: name },
      { onSuccess: () => setTriggeredName(name) },
    );
  };

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
  );
}
