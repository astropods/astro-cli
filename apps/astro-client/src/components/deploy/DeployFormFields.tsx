import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { Camera } from "lucide-react";
import { useAccountUsage } from "@/api/queries/usage";
import { accountSettingsPath } from "@/lib/settings-paths";
import { RequestIncreaseDialog } from "@/components/RequestIncreaseDialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AccountPicker } from "./AccountPicker";
import { InterfacesPicker } from "./InterfacesPicker";
import { CustomInterfacePicker } from "./CustomInterfacePicker";
import { VariableFields } from "./VariableFields";
import { FormSection } from "./FormSection";
import { ErrorPanel } from "@/components/ui/status-panel";
import { ImportVariables } from "./ImportVariables";
import { SchedulePicker } from "./SchedulePicker";
import { KnowledgeBindingPicker } from "./KnowledgeBindingPicker";
import { AdvancedProvisioningFields } from "./AdvancedProvisioningFields";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { useKnowledgeStores } from "@/api/queries/knowledge";
import type { useDeployForm } from "./useDeployForm";
import { slugToTitle } from "./useDeployForm";

type DeployForm = ReturnType<typeof useDeployForm>;

export interface DeployFormFieldsProps {
  form: DeployForm;
  /** Hide the account picker (e.g. on settings page where account is fixed). */
  hideAccountPicker?: boolean;
  /** Extra content rendered at the end of the Ingestion section (e.g. manual trigger buttons). */
  ingestionExtra?: ReactNode;
  /** Avatar display and optional upload/staging. */
  avatar?: {
    url?: string;
    account: string;
    blueprintName: string;
    /** Immediate upload (for existing deployments). */
    onUpload?: (file: Blob) => Promise<void>;
    isPending?: boolean;
    /** Stage a blob for deferred upload (for new deployments). */
    onStage?: (blob: Blob | null) => void;
    /** Local preview URL for a staged blob. */
    stagedPreviewUrl?: string;
  };
}

export function DeployFormFields({ form, hideAccountPicker, ingestionExtra, avatar }: DeployFormFieldsProps) {
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const [quotaDialogOpen, setQuotaDialogOpen] = useState(false);
  const deployNameErrorId = "agent-name-error";
  const { data: usageData } = useAccountUsage(form.targetAccount);
  // Settings → Usage lives at a different path for personal vs. organization
  // accounts. Scope the link to the deploy target (from form.accounts, which
  // already holds it) rather than the user's personal account.
  const usageSettingsPath = accountSettingsPath(form.accounts, form.targetAccount, "usage");
  const hasKnowledgeEntries = form.knowledgeEntries && Object.keys(form.knowledgeEntries).length > 0;
  const { data: knowledgeStores } = useKnowledgeStores(form.targetAccount, hasKnowledgeEntries);
  const computeMeter = usageData?.meters?.compute ?? { usage: 0, quota: undefined };
  const isAtComputeLimit = computeMeter.quota != null && computeMeter.usage >= computeMeter.quota;
  const showComputeLimit = isAtComputeLimit || (!!form.deployError && /compute limit/i.test(form.deployError.message));
  const importableKeys = new Set<string>([
    ...form.requiredVariables.map(([key]) => key),
    ...form.optionalVariables.map(([key]) => key),
    ...Object.values(form.adapterDisplayFields).flatMap((defs) => defs.map(([key]) => key)),
  ]);
  const showImport = importableKeys.size > 1;
  if (form.templateErrorMessage) {
    return <ErrorPanel>{form.templateErrorMessage}</ErrorPanel>;
  }

  if (!form.template) return null;

  return (
    <div className="space-y-12">
      {/* Agent name & account */}
      <FormSection title="General" description="Choose what to call your agent and where to deploy it.">
        <div className="space-y-5">
          <div className="flex items-start gap-4">
            {avatar && (() => {
              const canEdit = !!avatar.onUpload || !!avatar.onStage;
              const avatarImage = avatar.stagedPreviewUrl ? (
                <img
                  src={avatar.stagedPreviewUrl}
                  alt={avatar.blueprintName}
                  className="size-[68px] rounded-sm object-cover"
                />
              ) : (
                <BlueprintIdentity
                  account={avatar.account}
                  name={avatar.blueprintName}
                  url={avatar.url}
                  size={68}
                  className="size-[68px] rounded-sm overflow-hidden"
                />
              );
              const handleUploadOrStage = async (blob: Blob) => {
                if (avatar.onUpload) {
                  await avatar.onUpload(blob);
                } else if (avatar.onStage) {
                  avatar.onStage(blob);
                }
              };
              return canEdit ? (
                <>
                  <button
                    type="button"
                    aria-label="Edit agent avatar"
                    className="group relative shrink-0 cursor-pointer"
                    onClick={() => setAvatarDialogOpen(true)}
                  >
                    {avatarImage}
                    <div className="absolute inset-0 flex items-center justify-center rounded-sm bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                      <Camera className="size-5 text-white" />
                    </div>
                  </button>
                  <AvatarUploadDialog
                    open={avatarDialogOpen}
                    onOpenChange={setAvatarDialogOpen}
                    onUpload={handleUploadOrStage}
                    isPending={avatar.isPending ?? false}
                    title="Upload agent image"
                    cropShape="rect"
                  />
                </>
              ) : (
                <div className="shrink-0">{avatarImage}</div>
              );
            })()}
            <div className="min-w-0 flex-1 max-w-[32rem]">
              <Label size="md">Agent Name</Label>
              <Input
                value={form.deployName}
                onChange={(e) => form.setDeployName(e.target.value)}
                placeholder="My Agent"
                aria-invalid={!!form.errors.deployName || undefined}
                aria-describedby={form.errors.deployName ? deployNameErrorId : undefined}
              />
              {form.errors.deployName && (
                <p id={deployNameErrorId} className="mt-1.5 text-xs text-destructive">
                  {form.errors.deployName}
                </p>
              )}
            </div>
          </div>

          {!hideAccountPicker && form.accounts.length > 1 && (
            <div>
              <Label size="md">Deploy to</Label>
              <AccountPicker
                accounts={form.accounts}
                selected={form.targetAccount}
                onChange={form.setTargetAccount}
              />
            </div>
          )}
        </div>
      </FormSection>

      {/* Interfaces — hidden when the agent declares only a custom frontend
          (no messaging adapters to pick from). */}
      {form.messagingSupported && (
        <FormSection title="Messaging interface" description="Choose how you want to interact with the agent.">
          <InterfacesPicker
            selected={form.selectedAdapters}
            onChange={form.setSelectedAdapters}
            adapterCredDefs={form.adapterDisplayFields}
            adapterCredentials={form.adapterCredentials}
            onAdapterCredentialsChange={form.setAdapterCredentials}
            showError={!!form.errors.adapters}
            adapterErrorKeys={form.errors.adapterCredentials}
            credentialLayoutByAdapter={{ web: "inline-card", slack: "inline-card" }}
            webGrants={form.webGrants}
            onWebGrantsChange={form.setWebGrants}
            slackGrants={form.slackGrants}
            onSlackGrantsChange={form.setSlackGrants}
            targetAccount={form.targetAccount}
            vaultEntries={form.vaultEntries}
            vaultEntriesLoaded={form.vaultEntriesLoaded}
            vaultSettingsUrl={form.vaultSettingsUrl}
            vaultLoadError={form.vaultEntriesLoadError}
            bulkSetVariables={form.bulkSetVariables}
          />
        </FormSection>
      )}

      {form.customSupported && (
        <FormSection title="Custom interface" description="Control access to the web UI your agent serves.">
          <CustomInterfacePicker
            isPublic={form.customPublic}
            onPublicChange={form.setCustomPublic}
            grants={form.customGrants}
            onGrantsChange={form.setCustomGrants}
            targetAccount={form.targetAccount}
          />
        </FormSection>
      )}

      {/* Knowledge bindings */}
      {hasKnowledgeEntries && (
        <FormSection title="Knowledge" description="Connect a knowledge store to give your agent access to indexed data.">
          <KnowledgeBindingPicker
            entries={form.knowledgeEntries!}
            bindings={form.knowledgeBindings}
            modes={form.knowledgeBindingModes}
            resolvedBindings={form.resolvedBindings}
            errorKeys={form.errors.knowledgeBindings}
            onChange={form.setKnowledgeBindings}
            onModeChange={form.setKnowledgeBindingMode}
            stores={knowledgeStores ?? []}
          />
        </FormSection>
      )}

      {/* Required variables */}
      {form.requiredVariables.length > 0 && (
        <FormSection
          title="Configuration"
          description="Required configuration for this agent."
          action={
            showImport
              ? <ImportVariables onImport={(values) => form.bulkSetVariables(values)} />
              : undefined
          }
        >
          <VariableFields
            variables={form.requiredVariables}
            values={form.variableValues}
            onChange={form.setVariableValues}
            errorKeys={form.errors.credentials}
            invalidRefKeys={form.invalidVaultRefKeys}
            account={form.targetAccount}
            vaultEntries={form.vaultEntries}
            vaultEntriesLoaded={form.vaultEntriesLoaded}
            vaultSettingsUrl={form.vaultSettingsUrl}
            vaultLoadError={form.vaultEntriesLoadError}
            bulkSetVariables={form.bulkSetVariables}
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
            invalidRefKeys={form.invalidVaultRefKeys}
            account={form.targetAccount}
            vaultEntries={form.vaultEntries}
            vaultEntriesLoaded={form.vaultEntriesLoaded}
            vaultSettingsUrl={form.vaultSettingsUrl}
            vaultLoadError={form.vaultEntriesLoadError}
            bulkSetVariables={form.bulkSetVariables}
          />
        </FormSection>
      )}

      {/* Advanced provisioning (collapsed by default) — every agent gets a
          persistent disk by default, so storage is always shown here rather
          than behind an enable/disable toggle. */}
      <FormSection title="Resources" description="Override the default compute and storage allocations.">
        <AdvancedProvisioningFields
          cpu={form.agentCpu}
          memory={form.agentMemory}
          mountPath={form.agentVolumeMount}
          storageSize={form.agentStorageSize}
          storageLocked={form.volumeAlreadyProvisioned}
          responseTimeout={form.agentResponseTimeout}
          onCpuChange={form.setAgentCpu}
          onMemoryChange={form.setAgentMemory}
          onMountPathChange={form.setAgentVolumeMount}
          onStorageSizeChange={form.setAgentStorageSize}
          onResponseTimeoutChange={form.setAgentResponseTimeout}
        />
      </FormSection>

      {/* Ingestion schedules + manual triggers */}
      {(form.scheduleIngestions.length > 0 || ingestionExtra) && (
        <FormSection title="Ingestion" description="Configure scheduled and manual ingestion jobs.">
          {form.scheduleIngestions.length > 0 && (
            <div className="space-y-6">
              {form.scheduleIngestions.map((name) => (
                <SchedulePicker
                  key={name}
                  label={slugToTitle(name)}
                  value={form.ingestionSchedules[name] ?? ""}
                  onChange={(cron) =>
                    form.setIngestionSchedules((prev) => ({ ...prev, [name]: cron }))
                  }
                  error={
                    form.errors.ingestionSchedules?.includes(name)
                      ? "A schedule is required"
                      : undefined
                  }
                />
              ))}
            </div>
          )}
          {ingestionExtra}
        </FormSection>
      )}

      {/* Error — rendered last so it sits just above the action bar */}
      {showComputeLimit ? (
        <>
          <ErrorPanel title="Compute limit reached">
            All compute hours for this billing period have been used. Review your{" "}
            <Link
              to={usageSettingsPath}
              className="underline underline-offset-2 font-medium cursor-pointer"
            >
              usage in Settings
            </Link>{" "}
            or{" "}
            <button
              type="button"
              className="underline underline-offset-2 font-medium cursor-pointer"
              onClick={() => setQuotaDialogOpen(true)}
            >
              request a quota increase
            </button>.
          </ErrorPanel>
          <RequestIncreaseDialog
            featureKey="compute"
            label="Compute unit hours"
            meter={computeMeter}
            account={form.targetAccount}
            open={quotaDialogOpen}
            onOpenChange={setQuotaDialogOpen}
          />
        </>
      ) : form.deployError ? (
        <ErrorPanel title={form.deployError.message}>
          {form.deployError.details ?? null}
        </ErrorPanel>
      ) : form.vaultEntriesLoadError ? (
        <ErrorPanel title="Couldn't load your variables">
          {form.vaultEntriesLoadError}
        </ErrorPanel>
      ) : null}
    </div>
  );
}
