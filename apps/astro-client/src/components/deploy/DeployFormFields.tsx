import { useState, type ReactNode } from "react";
import { Camera } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AccountPicker } from "./AccountPicker";
import { InterfacesPicker } from "./InterfacesPicker";
import { VariableFields } from "./VariableFields";
import { FormSection } from "./FormSection";
import { ErrorPanel } from "@/components/ui/status-panel";
import { ImportVariables } from "./ImportVariables";
import { SchedulePicker } from "./SchedulePicker";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
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
              const displayUrl = avatar.stagedPreviewUrl ?? avatar.url;
              const avatarImage = displayUrl ? (
                <img
                  src={displayUrl}
                  alt={avatar.blueprintName}
                  className="size-[68px] rounded-sm object-cover"
                />
              ) : (
                <BlueprintIdentity
                  account={avatar.account}
                  name={avatar.blueprintName}
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
            <div className="flex-1 min-w-0">
              <Label size="md">Agent Name</Label>
              <Input
                value={form.deployName}
                onChange={(e) => form.setDeployName(e.target.value)}
                placeholder="My Agent"
                maxLength={64}
                aria-invalid={!!form.errors.deployName}
              />
              {form.errors.deployName && (
                <p className="text-sm text-destructive mt-1">{form.errors.deployName}</p>
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

      {/* Interfaces */}
      <FormSection title="Messaging" description="Choose how you want to interact with the agent.">
        <InterfacesPicker
          selected={form.selectedAdapters}
          onChange={form.setSelectedAdapters}
          adapterCredDefs={form.adapterDisplayFields}
          adapterCredentials={form.adapterCredentials}
          onAdapterCredentialsChange={form.setAdapterCredentials}
          showError={!!form.errors.adapters}
          adapterErrorKeys={form.errors.adapterCredentials}
        />
      </FormSection>

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
          />
        </FormSection>
      )}

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

      {/* Error */}
      {form.deployError && (
        <ErrorPanel title="Deployment failed">{form.deployError}</ErrorPanel>
      )}
    </div>
  );
}
