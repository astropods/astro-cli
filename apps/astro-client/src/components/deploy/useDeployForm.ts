import { useState, useEffect, useMemo } from "react";
import type { ReactNode } from "react";
import { useDeploymentTemplate, useDeployAgent } from "@/api/queries/agents";
import { useAuth } from "@/lib/auth";
import type { DeploymentTemplate, DeploymentVariable, DeploymentSpec, ApiError } from "@/lib/api";

export interface UseDeployFormOptions {
  initialTemplate?: DeploymentTemplate;
}

// --- Adapter configuration (must match server) ---

export interface Adapter {
  id: string;
  label: string;
  description: string;
  icon?: ReactNode;
}

export const AVAILABLE_ADAPTERS: Adapter[] = [
  { id: "slack", label: "Slack", description: "Post messages and respond in channels" },
  { id: "web", label: "Web", description: "Browser-based chat interface" },
];

export const ADAPTER_CREDENTIALS: Record<string, { key: string; label: string; description: string; secret?: boolean; placeholder?: string; helpUrl?: string }[]> = {
  slack: [
    { key: "SLACK_BOT_TOKEN", label: "Slack Bot Token", description: "Slack bot token for messaging", secret: true, placeholder: "your-slack-bot-token", helpUrl: "https://docs.slack.dev/authentication/tokens/" },
    { key: "SLACK_APP_TOKEN", label: "Slack App Token", description: "Slack app token for socket mode", secret: true, placeholder: "your-slack-app-token", helpUrl: "https://docs.slack.dev/authentication/tokens/" },
  ],
};

function fulfillTemplate(
  template: DeploymentTemplate,
  variableValues: Record<string, string>,
  selectedAdapters: string[],
): DeploymentSpec {
  // Destructure out editable (template-only) so it is not present in the fulfilled spec
  const { editable: _editable, ...rest } = template;

  // Rebuild variables: keep only runtime fields, fill in user-supplied value
  const variables: Record<string, DeploymentVariable> = rest.variables
    ? Object.fromEntries(
        Object.entries(rest.variables).map(([key, { targets, secret, optional }]) => [
          key,
          { value: variableValues[key] ?? '', targets, secret, optional },
        ]),
      )
    : {};
  // Inject adapter credentials not already declared in template variables
  for (const adapterId of selectedAdapters) {
    const creds = ADAPTER_CREDENTIALS[adapterId];
    if (!creds) continue;
    for (const cred of creds) {
      if (!(cred.key in variables)) {
        variables[cred.key] = {
          value: variableValues[cred.key] ?? '',
          targets: [`interface.${adapterId}`],
          secret: cred.secret ?? false,
          optional: false,
        };
      }
    }
  }

  return {
    ...rest,
    spec: 'deployment/v1',
    variables: Object.keys(variables).length > 0 ? variables : undefined,
    interfaces: rest.interfaces
      ? { ...rest.interfaces, adapters: selectedAdapters }
      : rest.interfaces,
  };
}

// --- Validation errors ---

export interface FormErrors {
  adapters?: string;
  credentials?: string[];
  adapterCredentials?: string[];
}

// --- Hook ---

export function useDeployForm(account: string, name: string, opts?: UseDeployFormOptions) {
  const { accounts } = useAuth();
  const userAccount = accounts[0]?.name ?? "";

  const {
    data: template,
    isLoading: templateLoading,
    error: templateError,
  } = useDeploymentTemplate(account, name, { initialData: opts?.initialTemplate });

  const deployMutation = useDeployAgent(userAccount);

  const [variableValues, setVariableValues] = useState<Record<string, string>>({});
  const [selectedAdapters, setSelectedAdapters] = useState<string[]>(["web"]);
  const [adapterCredentials, setAdapterCredentials] = useState<Record<string, string>>({});
  const [deployError, setDeployError] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);

  // Derived variable lists (agent/ingestion-targeting variables)
  const variableEntries = useMemo(
    () =>
      template?.variables
        ? Object.entries(template.variables).filter(([, v]) =>
            v.targets.some((t) => t === "agent" || t.startsWith("ingestion")),
          )
        : [],
    [template],
  );
  const requiredVariables = useMemo(
    () => variableEntries.filter(([, v]) => !v.optional),
    [variableEntries],
  );
  const optionalVariables = useMemo(
    () => variableEntries.filter(([, v]) => v.optional),
    [variableEntries],
  );

  // Adapter credentials needed for selected adapters, derived from template.variables
  const selectedAdapterCreds = useMemo(
    () =>
      selectedAdapters.flatMap((id) => {
        if (template?.variables) {
          // Derive from template variables whose targets include "interface.<id>"
          const derived = Object.entries(template.variables).filter(([, v]) =>
            v.targets.some((t) => t === `interface.${id}`),
          );
          if (derived.length > 0) return derived;
        }
        // Fall back to ADAPTER_CREDENTIALS for UI metadata
        const creds = ADAPTER_CREDENTIALS[id];
        if (!creds) return [];
        return creds.map((c): [string, DeploymentVariable] => [
          c.key,
          { targets: [`interface.${id}`], description: c.description, optional: false },
        ]);
      }),
    [selectedAdapters, template],
  );

  // Initialize variable values when template loads
  useEffect(() => {
    if (variableEntries.length > 0) {
      setVariableValues((prev) => {
        const initial: Record<string, string> = {};
        for (const [key, v] of variableEntries) {
          initial[key] = prev[key] ?? v.default ?? "";
        }
        return initial;
      });
    }
  }, [variableEntries]);

  // Compute validation errors (only surfaced after first submit attempt)
  const errors = useMemo<FormErrors>(() => {
    if (!submitted) return {};

    const result: FormErrors = {};

    if (selectedAdapters.length === 0) {
      result.adapters = "Select at least one messaging type";
    }

    const emptyRequired = requiredVariables
      .filter(([key]) => !variableValues[key]?.trim())
      .map(([key]) => key);
    if (emptyRequired.length > 0) {
      result.credentials = emptyRequired;
    }

    const emptyAdapterCreds = selectedAdapters.flatMap((adapterId) => {
      const creds = ADAPTER_CREDENTIALS[adapterId];
      if (!creds) return [];
      return creds
        .filter((c) => !adapterCredentials[c.key]?.trim())
        .map((c) => c.key);
    });
    if (emptyAdapterCreds.length > 0) {
      result.adapterCredentials = emptyAdapterCreds;
    }

    return result;
  }, [submitted, selectedAdapters, requiredVariables, variableValues, adapterCredentials]);

  const isValid = submitted
    ? !errors.adapters && !errors.credentials && !errors.adapterCredentials
    : true;

  // Try to submit: marks form as submitted and returns validity
  const trySubmit = (): boolean => {
    setSubmitted(true);

    // Compute validity inline (state update is async, can't rely on `errors` yet)
    const hasAdapter = selectedAdapters.length > 0;
    const varsValid = requiredVariables.every(([key]) => variableValues[key]?.trim());
    const adapterCredsValid = selectedAdapters.every((adapterId) => {
      const creds = ADAPTER_CREDENTIALS[adapterId];
      if (!creds) return true;
      return creds.every((c) => adapterCredentials[c.key]?.trim());
    });

    return hasAdapter && varsValid && adapterCredsValid;
  };

  // Submission
  const deploy = async () => {
    if (!template || !account || !name) return;

    setDeployError(null);
    const allVariableValues = { ...variableValues, ...adapterCredentials };
    const spec = fulfillTemplate(template, allVariableValues, selectedAdapters);

    try {
      await deployMutation.mutateAsync(spec);
    } catch (err) {
      const apiErr = err as ApiError;
      const messages: string[] = [];
      if (apiErr.validation_errors?.length) {
        for (const ve of apiErr.validation_errors) messages.push(`${ve.field}: ${ve.message}`);
      }
      if (apiErr.missing_variables?.length) {
        messages.push(`Missing variables: ${apiErr.missing_variables.join(", ")}`);
      }
      if (messages.length === 0) {
        messages.push(
          apiErr.details ?? apiErr.error ?? (err instanceof Error ? err.message : "Deployment failed"),
        );
      }
      setDeployError(messages.join("\n"));
      throw err;
    }
  };

  const templateErrorMessage = templateError
    ? ((templateError as ApiError).error_description ??
      (templateError instanceof Error ? templateError.message : null) ??
      "Failed to load deployment configuration")
    : null;

  return {
    template,
    templateLoading,
    templateErrorMessage,

    selectedAdapters,
    setSelectedAdapters,
    selectedAdapterCreds,
    adapterCredentials,
    setAdapterCredentials,

    variableValues,
    setVariableValues,
    requiredVariables,
    optionalVariables,

    errors,
    submitted,
    isValid,
    trySubmit,
    deploy,
    isDeploying: deployMutation.isPending,
    deployError,
  };
}
