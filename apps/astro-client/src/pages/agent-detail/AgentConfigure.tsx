import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { Loader2, Rocket, Save, History, X, Play, Check } from "lucide-react";
import { motion } from "motion/react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { useAgentDetailContext } from "../AgentDetail";
import { useDeployForm, slugToTitle } from "@/components/deploy/useDeployForm";
import { DeployFormFields } from "@/components/deploy/DeployFormFields";
import { BlueprintVersionPicker } from "@/components/deploy/BlueprintVersionPicker";
import { useBlueprint } from "@/api/queries/blueprints";
import { useUploadDeploymentAvatar, useUpdateDeploymentDisplayName, useTriggerIngestion, useWakeUpDeployment, useDeploymentStatus } from "@/api/queries/deployments";
import { isPausedState } from "@/lib/deployment-utils";
import { bustDeploymentAvatar, useDeploymentAvatarBust } from "@/lib/avatar-bust";
import { deploymentKeys } from "@/api/queries/keys";
import { useContentRevealMotion } from "@/components/ui/content-reveal-motion";

const FORM_ID = "agent-configure-form";

export default function AgentConfigure() {
  const contentRevealMotion = useContentRevealMotion();
  const { deployment, runtime, account, deploymentId } = useAgentDetailContext();
  const manualIngestions = runtime?.manual_ingestions ?? [];
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const rollbackRevision = searchParams.get("revision");
  const rollbackBuild = searchParams.get("build");
  const isRollback = rollbackRevision !== null;
  const isBuildHandoff = !isRollback && rollbackBuild !== null;
  const [selectedBuildOverride, setSelectedBuildOverride] = useState<string | null>(() =>
    isBuildHandoff && rollbackBuild !== deployment.build_id ? rollbackBuild : null,
  );
  const isBuildOverride = !isRollback && selectedBuildOverride !== null;
  const hasOverride = isRollback || isBuildOverride;
  const selectedBuildId = rollbackBuild ?? selectedBuildOverride ?? deployment.build_id;
  const sourceAccount = deployment.source_account || account;
  const blueprintReadable = sourceAccount === account || !!deployment.latest_build_id;
  const { data: blueprint, isLoading: versionsLoading } = useBlueprint(
    sourceAccount,
    deployment.name,
    { enabled: blueprintReadable },
  );

  const formOpts = useMemo(() => ({
    deploymentId: deployment.id,
    build: selectedBuildId,
    // Pin redeploys to the deployment's account.
    allowedTargetAccounts: account ? [account] : undefined,
    ...(rollbackRevision ? { revision: Number(rollbackRevision) } : {}),
  }), [account, deployment.id, rollbackRevision, selectedBuildId]);

  const form = useDeployForm(account, deployment.name, formOpts);

  const uploadAvatar = useUploadDeploymentAvatar(account);
  const avatarBust = useDeploymentAvatarBust(deployment.id);
  const renameMutation = useUpdateDeploymentDisplayName(deployment.id);
  const wakeupMutation = useWakeUpDeployment(account);
  const { data: liveStatus } = useDeploymentStatus(deploymentId);
  // A paused agent can't redeploy; explain why and offer Resume. Prefer the live
  // status (flips on resume) over the record, which lags until it refetches.
  const paused = liveStatus ? liveStatus.value === "inactive" : isPausedState(deployment);
  const handleResume = useCallback(() => {
    wakeupMutation.mutate({ deploymentId: deployment.id });
  }, [wakeupMutation, deployment.id]);

  // Build links are one-time handoffs into upgrade mode. Keeping the temporary
  // selection out of the URL makes refreshes and future tab visits start from
  // the agent's currently deployed build.
  useEffect(() => {
    if (isBuildHandoff) {
      setSearchParams({}, { replace: true });
    }
  }, [isBuildHandoff, setSearchParams]);

  const clearOverrideParams = useCallback(() => {
    if (hasOverride) {
      setSelectedBuildOverride(null);
      setSearchParams({}, { replace: true });
    }
  }, [hasOverride, setSearchParams]);

  const handleBuildChange = useCallback((buildId: string) => {
    setSelectedBuildOverride(buildId === deployment.build_id ? null : buildId);
    setSearchParams({}, { replace: true });
  }, [deployment.build_id, setSearchParams]);

  const navigateToDeployments = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: deploymentKeys.detail(deploymentId) });
    void navigate("../deployments", { relative: "path" });
  }, [queryClient, deploymentId, navigate]);

  const isBusy = form.isDeploying || renameMutation.isPending || form.templateLoading;
  const hasTemplateSwitchError = !!form.templateError && hasOverride;

  const isNameOnly = form.nameChanged && !form.deployChanged && !hasOverride;

  const handleSubmit = async (e: React.SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (isBusy) return;
    if (hasTemplateSwitchError) return;
    if (isNameOnly) {
      if (form.errors.deployName) return;
      try {
        await renameMutation.mutateAsync(form.deployName);
        form.reset({ ...form.initialValues!, deployName: form.deployName });
      } catch {
        // mutation error available via renameMutation.error
      }
    } else {
      if (!form.trySubmit()) return;
      try {
        const result = await form.deploy();
        if (!result) return;
        navigateToDeployments();
      } catch {
        // captured in form.deployError and rendered inline
      }
    }
  };

  const handleDiscard = useCallback(() => {
    if (isBusy) return;
    if (hasOverride) {
      clearOverrideParams();
    } else {
      form.reset();
    }
  }, [form, isBusy, hasOverride, clearOverrideParams]);

  // Wait for initialValues — Radix Select wipes its value on the "" → seeded transition.
  if (!form.template || !form.initialValues) {
    return (
      <div className="relative z-10 flex flex-1 items-center justify-center">
        {form.templateErrorMessage ? (
          <p className="text-body-sm text-destructive">{form.templateErrorMessage}</p>
        ) : (
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        )}
      </div>
    );
  }

  return (
    <div className="relative z-10 flex flex-1 flex-col pt-16">
      {/* Scrollable form area */}
      <div
        className="@container/scroll relative z-10 min-h-0 flex-1 overflow-y-auto"
        style={{ maskImage: "linear-gradient(to bottom, transparent, black 2rem)", WebkitMaskImage: "linear-gradient(to bottom, transparent, black 2rem)" }}
      >
        <motion.form
          {...contentRevealMotion}
          id={FORM_ID}
          onSubmit={handleSubmit}
          className="mx-auto w-full max-w-3xl px-6 py-8 pb-32 @max-[600px]/scroll:pb-44 @max-[400px]/scroll:pb-56"
        >
          <BlueprintVersionPicker
            versions={blueprint?.versions ?? []}
            selectedBuildId={selectedBuildId}
            latestBuildId={deployment.latest_build_id}
            currentBuildId={deployment.build_id}
            onBuildChange={handleBuildChange}
            loading={versionsLoading || form.templateLoading}
            error={hasTemplateSwitchError ? form.templateError : undefined}
            recovery={{ label: "Use current build", onClick: clearOverrideParams }}
          />
          {isRollback && (
            <div className="mb-6 flex items-center gap-3 rounded-md border border-indigo-600/30 bg-indigo-300/80 px-4 py-3 dark:border-indigo-500/20 dark:bg-indigo-500/18">
              <History className="size-4 shrink-0 text-indigo-700 dark:text-indigo-300" />
              <div className="flex min-w-0 flex-1 items-center gap-2 text-body-sm text-indigo-950 dark:text-indigo-100">
                <span className="font-medium">Rollback</span>
                <span className="text-indigo-950/50 dark:text-indigo-100/50">·</span>
                <span>Config #{rollbackRevision}</span>
                {rollbackBuild && (
                  <>
                    <span className="text-indigo-950/50 dark:text-indigo-100/50">·</span>
                    <span className="font-mono">{rollbackBuild.slice(0, 8)}</span>
                  </>
                )}
              </div>
              <button
                type="button"
                className="shrink-0 rounded p-0.5 text-indigo-700/60 transition-colors hover:text-indigo-900 dark:text-indigo-300/60 dark:hover:text-indigo-100"
                onClick={clearOverrideParams}
              >
                <X className="size-3.5" />
              </button>
            </div>
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
              hideAccountPicker
              avatar={{
                url: avatarBust ?? deployment.avatar_url,
                account,
                blueprintName: deployment.name,
                onUpload: async (file) => {
                  await uploadAvatar.mutateAsync({ id: deployment.id, file });
                  bustDeploymentAvatar(deployment.id, file);
                },
                isPending: uploadAvatar.isPending,
              }}
              ingestionExtra={
                manualIngestions.length > 0
                  ? <ManualTriggers deploymentId={deployment.id} names={manualIngestions} account={account} hasBorderTop={form.scheduleIngestions.length > 0} />
                  : undefined
              }
            />
          </fieldset>
        </motion.form>
      </div>

      {/* Footer gradient mask */}
      <motion.div
        className="pointer-events-none absolute inset-x-0 bottom-0 z-10 h-24 bg-[linear-gradient(to_bottom,transparent_0%,var(--color-surface)_50%)] dark:bg-[linear-gradient(to_bottom,transparent_0%,var(--color-background)_50%)]"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.3, ease: "easeOut" }}
      />

      {/* Floating footer — always visible action bar */}
      <motion.div
        className="pointer-events-none absolute inset-x-3 bottom-4 z-20 flex justify-center"
        initial={{ y: "calc(100% + 1rem)" }}
        animate={{ y: 0 }}
        transition={{ type: "spring", bounce: 0.15, duration: 0.5 }}
      >
        <div className="@container/footer pointer-events-auto w-full max-w-[52rem] rounded-lg border border-border bg-surface/95 py-3 pl-5 pr-3 shadow-lg backdrop-blur @max-[600px]/footer:px-5 supports-[backdrop-filter]:bg-surface/90">
          <div className="flex items-center gap-3 @max-[600px]/footer:flex-col @max-[600px]/footer:gap-3">
            <p className="text-body text-muted-foreground @max-[600px]/footer:text-center">
              {paused
                ? "This agent is paused, so it can't be redeployed. Resume it to deploy again."
                : isRollback
                ? `Rollback to config #${rollbackRevision}. Review and redeploy.`
                : isBuildOverride
                ? "Redeploy with the selected build."
                : form.deployChanged
                ? `Redeploy to apply ${form.changeCount} ${form.changeCount === 1 ? "change" : "changes"}.`
                : isNameOnly
                ? "Save to update the agent name."
                : "Redeploy the current configuration."}
            </p>
            <div className="ml-auto flex shrink-0 items-center gap-1.5 @max-[600px]/footer:ml-0 @max-[600px]/footer:w-full @max-[400px]/footer:flex-col">
              {paused && (
                <Button
                  type="button"
                  variant="default"
                  className="shrink-0 @max-[600px]/footer:flex-1 @max-[400px]/footer:w-full @max-[400px]/footer:flex-none"
                  disabled={wakeupMutation.isPending}
                  onClick={handleResume}
                >
                  {wakeupMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Play className="size-3.5" />}
                  {wakeupMutation.isPending ? "Resuming…" : "Resume to deploy"}
                </Button>
              )}
              {!paused && (form.isDirty || hasOverride) && (
                <Button
                  type="button"
                  variant="ghost"
                  className="shrink-0 @max-[600px]/footer:flex-1 @max-[600px]/footer:border @max-[600px]/footer:border-border @max-[400px]/footer:w-full @max-[400px]/footer:flex-none"
                  disabled={isBusy}
                  onClick={handleDiscard}
                >
                  Discard
                </Button>
              )}
              {paused ? null : isNameOnly ? (
                <Button
                  type="submit"
                  form={FORM_ID}
                  variant="default"
                  className="shrink-0 @max-[600px]/footer:flex-1 @max-[400px]/footer:w-full @max-[400px]/footer:flex-none"
                  disabled={isBusy}
                >
                  {renameMutation.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Save className="size-3.5" />}
                  {renameMutation.isPending ? "Saving…" : "Save"}
                </Button>
              ) : (
                <Button
                  type="submit"
                  form={FORM_ID}
                  variant="default"
                  className="shrink-0 @max-[600px]/footer:flex-1 @max-[400px]/footer:w-full @max-[400px]/footer:flex-none"
                  disabled={isBusy}
                >
                  {form.isDeploying ? <Loader2 className="size-3.5 animate-spin" /> : <Rocket className="size-3.5" />}
                  {form.isDeploying ? "Redeploying…" : "Redeploy"}
                </Button>
              )}
            </div>
          </div>
        </div>
      </motion.div>
    </div>
  );
}

function ManualTriggers({ deploymentId, names, account, hasBorderTop }: { deploymentId: string; names: string[]; account: string; hasBorderTop: boolean }) {
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
                <Check className="size-3.5 text-green-600 dark:text-green-400" />
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
