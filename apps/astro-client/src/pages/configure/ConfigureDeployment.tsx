import { useEffect, useMemo } from "react";
import { useNavigate, useOutletContext } from "react-router";
import type { ConfigureContext } from "./types";
import { deploymentPath } from "@/lib/routes";
import { useDeployForm } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { DeployFormActionBar } from "@/components/deploy/DeployFormActionBar";
import { extractInitialValues } from "@/components/deploy/extractInitialValues";
import { useChangeTracking, type TrackedFormState } from "@/components/deploy/useChangeTracking";

const FORM_ID = "configure-deployment-form";

export default function ConfigureDeployment() {
  const { account, deployment, template } = useOutletContext<ConfigureContext>();
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
  };
  const initialTrackedState: TrackedFormState = {
    deployName: initialValues.deployName ?? "",
    variableValues: initialValues.variableValues ?? {},
    selectedAdapters: initialValues.selectedAdapters ?? ["web"],
    adapterCredentials: initialValues.adapterCredentials ?? {},
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
      <form id={FORM_ID} onSubmit={handleSubmit} className={changes.isDirty ? "pb-24" : ""}>
        <DeployFormFields form={form} hideAccountPicker />
      </form>

      <DeployFormActionBar
        isDirty={changes.isDirty}
        changeCount={changes.changeCount}
        requiresRedeploy={changes.requiresRedeploy}
        isSaving={form.isDeploying}
        formId={FORM_ID}
        onReset={() => form.reset(initialValues)}
      />
    </>
  );
}
