import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AccountPicker } from "./AccountPicker";
import { InterfacesPicker } from "./InterfacesPicker";
import { VariableFields } from "./VariableFields";
import { FormSection } from "./FormSection";
import { ErrorPanel } from "./ErrorPanel";
import type { useDeployForm } from "./useDeployForm";

type DeployForm = ReturnType<typeof useDeployForm>;

export interface DeployFormFieldsProps {
  form: DeployForm;
  /** Hide the account picker (e.g. on settings page where account is fixed). */
  hideAccountPicker?: boolean;
}

export function DeployFormFields({ form, hideAccountPicker }: DeployFormFieldsProps) {
  if (form.templateErrorMessage) {
    return <ErrorPanel>{form.templateErrorMessage}</ErrorPanel>;
  }

  if (!form.template) return null;

  return (
    <div className="space-y-12">
      {/* Agent name & account */}
      <FormSection title="General" description="Choose what to call your agent and where to install it.">
        <div className="space-y-5">
          <div>
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

          {!hideAccountPicker && form.accounts.length > 1 && (
            <div>
              <Label size="md">Install to</Label>
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
          adapterCredDefs={form.allAdapterCredDefs}
          adapterCredentials={form.adapterCredentials}
          onAdapterCredentialsChange={form.setAdapterCredentials}
          showError={!!form.errors.adapters}
          adapterErrorKeys={form.errors.adapterCredentials}
        />
      </FormSection>

      {/* Required variables */}
      {form.requiredVariables.length > 0 && (
        <FormSection title="Configuration" description="Required configuration for this agent.">
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

      {/* Error */}
      {form.deployError && (
        <ErrorPanel title="Deployment failed">{form.deployError}</ErrorPanel>
      )}
    </div>
  );
}
