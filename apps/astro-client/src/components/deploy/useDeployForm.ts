import { useState, useEffect, useMemo } from "react";
import type { ReactNode } from "react";
import { useDeploymentTemplate, useDeployAgent } from "@/api/queries/agents";
import { useAuth } from "@/lib/auth";
import type { DeploymentTemplate, DeploymentTemplateCredential, ApiError } from "@/lib/api";

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

export const ADAPTER_CREDENTIALS: Record<string, { key: string; label: string; description: string; placeholder?: string; helpUrl?: string }[]> = {
  slack: [
    { key: "SLACK_BOT_TOKEN", label: "Slack Bot Token", description: "Slack bot token for messaging", placeholder: "your-slack-bot-token", helpUrl: "https://docs.slack.dev/authentication/tokens/" },
    { key: "SLACK_APP_TOKEN", label: "Slack App Token", description: "Slack app token for socket mode", placeholder: "your-slack-app-token", helpUrl: "https://docs.slack.dev/authentication/tokens/" },
  ],
};

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

  const [credentialValues, setCredentialValues] = useState<Record<string, string>>({});
  const [selectedAdapters, setSelectedAdapters] = useState<string[]>(["web"]);
  const [adapterCredentials, setAdapterCredentials] = useState<Record<string, string>>({});
  const [deployError, setDeployError] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);

  // Derived credential lists
  const credentialEntries = useMemo(
    () => (template?.credentials ? Object.entries(template.credentials) : []),
    [template],
  );
  const requiredCredentials = useMemo(
    () => credentialEntries.filter(([, c]) => !c.optional),
    [credentialEntries],
  );
  const optionalCredentials = useMemo(
    () => credentialEntries.filter(([, c]) => c.optional),
    [credentialEntries],
  );

  // Adapter credentials needed for selected adapters
  const selectedAdapterCreds = useMemo(
    () =>
      selectedAdapters.flatMap((id) => {
        const creds = ADAPTER_CREDENTIALS[id];
        if (!creds) return [];
        return creds.map(
          (c) =>
            [c.key, { description: c.description, optional: false }] as [
              string,
              DeploymentTemplateCredential,
            ],
        );
      }),
    [selectedAdapters],
  );

  // Initialize credential values when template loads
  useEffect(() => {
    if (credentialEntries.length > 0) {
      setCredentialValues((prev) => {
        const initial: Record<string, string> = {};
        for (const [key] of credentialEntries) {
          initial[key] = prev[key] ?? "";
        }
        return initial;
      });
    }
  }, [credentialEntries]);

  // Compute validation errors (only surfaced after first submit attempt)
  const errors = useMemo<FormErrors>(() => {
    if (!submitted) return {};

    const result: FormErrors = {};

    if (selectedAdapters.length === 0) {
      result.adapters = "Select at least one messaging type";
    }

    const emptyRequired = requiredCredentials
      .filter(([key]) => !credentialValues[key]?.trim())
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
  }, [submitted, selectedAdapters, requiredCredentials, credentialValues, adapterCredentials]);

  const isValid = submitted
    ? !errors.adapters && !errors.credentials && !errors.adapterCredentials
    : true;

  // Try to submit: marks form as submitted and returns validity
  const trySubmit = (): boolean => {
    setSubmitted(true);

    // Compute validity inline (state update is async, can't rely on `errors` yet)
    const hasAdapter = selectedAdapters.length > 0;
    const credsValid = requiredCredentials.every(([key]) => credentialValues[key]?.trim());
    const adapterCredsValid = selectedAdapters.every((adapterId) => {
      const creds = ADAPTER_CREDENTIALS[adapterId];
      if (!creds) return true;
      return creds.every((c) => adapterCredentials[c.key]?.trim());
    });

    return hasAdapter && credsValid && adapterCredsValid;
  };

  // Submission
  const deploy = async () => {
    if (!account || !name) return;

    setDeployError(null);
    const allCredentials = { ...credentialValues, ...adapterCredentials };

    try {
      await deployMutation.mutateAsync({
        account: userAccount,
        name,
        source_account: account !== userAccount ? account : undefined,
        user_credentials: allCredentials,
        interfaces: selectedAdapters.length > 0 ? selectedAdapters : undefined,
      });
    } catch (err) {
      const apiErr = err as {
        error?: string;
        details?: string;
        validation_errors?: Array<{ field: string; message: string }>;
        missing_credentials?: string[];
      };

      const messages: string[] = [];
      if (apiErr.validation_errors?.length) {
        for (const ve of apiErr.validation_errors) messages.push(`${ve.field}: ${ve.message}`);
      }
      if (apiErr.missing_credentials?.length) {
        messages.push(`Missing credentials: ${apiErr.missing_credentials.join(", ")}`);
      }
      if (messages.length === 0) {
        messages.push(
          apiErr.details || apiErr.error || (err instanceof Error ? err.message : "Deployment failed"),
        );
      }
      setDeployError(messages.join("\n"));
      throw err;
    }
  };

  const templateErrorMessage = templateError
    ? ((templateError as unknown as ApiError).error_description ??
      (templateError as Error).message ??
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

    credentialValues,
    setCredentialValues,
    requiredCredentials,
    optionalCredentials,

    errors,
    submitted,
    isValid,
    trySubmit,
    deploy,
    isDeploying: deployMutation.isPending,
    deployError,
  };
}
