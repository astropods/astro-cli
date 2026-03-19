import { useState, useEffect, useMemo } from "react";
import type { ReactNode } from "react";
import { useDeploymentTemplate, useDeployAgent } from "@/api/queries/agents";
import { useAuth } from "@/lib/auth";
import type { DeploymentTemplate, DeploymentVariable, DeploymentSpec, DeployResponse, ApiError } from "@/lib/api";
import type { VariableDisplay } from "./VariableFields";
import { getVariableDefault, isVariableFilled } from "./VariableField";
import { SLACK_CONFIG_KEY, serializeSlackConfig, deserializeSlackConfig } from "./slackConfig";

export interface DeployFormInitialValues {
  deployName?: string;
  variableValues?: Record<string, string>;
  selectedAdapters?: string[];
  adapterCredentials?: Record<string, string>;
  targetAccount?: string;
  ingestionSchedules?: Record<string, string>;
}

export interface UseDeployFormOptions {
  initialTemplate?: DeploymentTemplate;
  /** Pre-fill form state (e.g. from an existing deployment's spec). */
  initialValues?: DeployFormInitialValues;
  /**
   * When true, skip fetching the template from the API and rely entirely
   * on `initialTemplate`. Used on the settings page where the template
   * is derived from the existing deployment.
   */
  skipTemplateFetch?: boolean;
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

export interface AdapterFieldDef {
  key: string;
  label: string;
  description: string;
  secret?: boolean;
  optional?: boolean;
  placeholder?: string;
  helpUrl?: string;
}

export const ADAPTER_SECRETS: Record<string, AdapterFieldDef[]> = {
  slack: [
    { key: "SLACK_BOT_TOKEN", label: "Slack Bot Token", description: "Slack bot token for messaging", secret: true, placeholder: "your-slack-bot-token", helpUrl: "https://docs.slack.dev/authentication/tokens/" },
    { key: "SLACK_APP_TOKEN", label: "Slack App Token", description: "Slack app token for socket mode", secret: true, placeholder: "your-slack-app-token", helpUrl: "https://docs.slack.dev/authentication/tokens/" },
  ],
};

export const ADAPTER_CONFIG: Record<string, AdapterFieldDef[]> = {
  slack: [
    { key: "SLACK_ACTIONABLE_REACTIONS", label: "Actionable Reactions", description: "Emoji names the bot acts on (comma-separated)", optional: true, placeholder: "ticket, bug" },
    { key: "SLACK_ALLOWED_CHANNEL_IDS", label: "Allowed Channel IDs", description: "Restrict access to specific Slack channel IDs (comma-separated)", optional: true, placeholder: "C12345, C67890" },
    { key: "SLACK_ALLOWED_USER_IDS", label: "Allowed User IDs", description: "Restrict access to specific Slack user IDs (comma-separated)", optional: true, placeholder: "U12345, U67890" },
  ],
};

export const adapterFields = (adapterId: string): AdapterFieldDef[] => [
  ...(ADAPTER_SECRETS[adapterId] ?? []),
  ...(ADAPTER_CONFIG[adapterId] ?? []),
];

function toVariableDisplay(v: DeploymentVariable): VariableDisplay {
  return {
    description: v.description,
    optional: v.optional,
    secret: v.secret,
    datatype: v.datatype,
    displayAs: v['display-as'],
    options: v.options,
    defaultValue: v.default,
  };
}

const hasTextValue = (value: string | undefined): boolean => !!value?.trim();

const mergeFormValues = (
  variableValues: Record<string, string>,
  adapterCredentials: Record<string, string>,
): Record<string, string> => {
  const keys = new Set([
    ...Object.keys(variableValues),
    ...Object.keys(adapterCredentials),
  ]);
  const merged: Record<string, string> = {};
  for (const key of keys) {
    const variableValue = variableValues[key];
    const adapterValue = adapterCredentials[key];
    if (hasTextValue(adapterValue)) {
      merged[key] = adapterValue;
      continue;
    }
    if (hasTextValue(variableValue)) {
      merged[key] = variableValue;
      continue;
    }
    merged[key] = adapterValue ?? variableValue ?? "";
  }
  return merged;
};

/** Convert a slug like "code-reviewer" to title case: "Code Reviewer" */
export function slugToTitle(slug: string): string {
  return slug
    .split(/[-_]+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

function fulfillTemplate(
  template: DeploymentTemplate,
  variableValues: Record<string, string>,
  selectedAdapters: string[],
  adapterVariableDefs: Record<string, [string, VariableDisplay][]>,
  targetAccount: string,
  deployName: string,
  ingestionSchedules: Record<string, string>,
): DeploymentSpec {
  // Destructure out editable (template-only) so it is not present in the fulfilled spec
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- intentionally omitting editable from rest
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
    const creds = adapterVariableDefs[adapterId] ?? [];
    for (const [key, cred] of creds) {
      if (!(key in variables)) {
        variables[key] = {
          value: variableValues[key] ?? '',
          targets: [`interface.${adapterId}`],
          secret: cred.secret ?? false,
          optional: cred.optional ?? false,
        };
      }
    }
  }

  // SLACK_CONFIG is stored as three virtual fields in the form; serialize them back
  if (SLACK_CONFIG_KEY in variables) {
    variables[SLACK_CONFIG_KEY] = {
      ...variables[SLACK_CONFIG_KEY],
      value: serializeSlackConfig(variableValues),
    };
  }

  // Merge user-supplied cron expressions into ingestion schedule triggers
  const ingestion = rest.ingestion
    ? Object.fromEntries(
        Object.entries(rest.ingestion).map(([name, ing]) => {
          if (ing.trigger?.type === "schedule" && name in ingestionSchedules) {
            return [name, { ...ing, trigger: { ...ing.trigger, schedule: ingestionSchedules[name] } }];
          }
          return [name, ing];
        }),
      )
    : rest.ingestion;

  return {
    ...rest,
    spec: 'deployment/v1',
    target: { ...rest.target, account: targetAccount, display_name: deployName },
    variables: Object.keys(variables).length > 0 ? variables : undefined,
    interfaces: rest.interfaces
      ? { ...rest.interfaces, adapters: selectedAdapters }
      : rest.interfaces,
    ingestion,
  };
}

// --- Validation errors ---

export interface FormErrors {
  account?: string;
  deployName?: string;
  adapters?: string;
  credentials?: string[];
  adapterCredentials?: string[];
  ingestionSchedules?: string[];
}

// --- Hook ---

export function useDeployForm(account: string, name: string, opts?: UseDeployFormOptions) {
  const { accounts, personalAccount } = useAuth();
  const iv = opts?.initialValues;

  const [targetAccount, setTargetAccount] = useState(iv?.targetAccount ?? personalAccount?.name ?? "");

  const {
    data: fetchedTemplate,
    isLoading: templateLoading,
    error: templateError,
  } = useDeploymentTemplate(account, name, {
    initialData: opts?.initialTemplate,
    enabled: !opts?.skipTemplateFetch,
  });

  const template = opts?.skipTemplateFetch ? (opts.initialTemplate ?? null) : fetchedTemplate;

  const deployMutation = useDeployAgent(targetAccount, name);

  const [deployName, setDeployName] = useState(() => iv?.deployName ?? slugToTitle(name));
  const [variableValues, setVariableValues] = useState<Record<string, string>>(iv?.variableValues ?? {});
  const [selectedAdapters, setSelectedAdapters] = useState<string[]>(iv?.selectedAdapters ?? ["web"]);
  const [adapterCredentials, setAdapterCredentials] = useState<Record<string, string>>(iv?.adapterCredentials ?? {});
  const [ingestionSchedules, setIngestionSchedules] = useState<Record<string, string>>(iv?.ingestionSchedules ?? {});
  const [deployError, setDeployError] = useState<string | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const allFormValues = useMemo(
    () => mergeFormValues(variableValues, adapterCredentials),
    [variableValues, adapterCredentials],
  );

  const scheduleIngestions = useMemo<string[]>(
    () =>
      template?.ingestion
        ? Object.entries(template.ingestion)
            .filter(([, ing]) => ing.trigger?.type === "schedule")
            .map(([name]) => name)
        : [],
    [template],
  );

  // Initialize ingestion schedule values when template loads
  useEffect(() => {
    if (scheduleIngestions.length > 0) {
      setIngestionSchedules((prev) => {
        const initial: Record<string, string> = {};
        for (const name of scheduleIngestions) {
          initial[name] = prev[name] ?? template?.ingestion?.[name]?.trigger?.schedule ?? "";
        }
        return initial;
      });
    }
  }, [scheduleIngestions, template]);

  // Derived variable lists (agent/ingestion-targeting variables)
  const variableEntries = useMemo<[string, VariableDisplay][]>(
    () =>
      template?.variables
        ? Object.entries(template.variables)
            .filter(([, v]) =>
              v.targets.some((t) => t === "agent" || t.startsWith("ingestion")),
            )
            .map(([key, v]): [string, VariableDisplay] => [key, toVariableDisplay(v)])
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

  // Two views of adapter fields: one for submission (real template variables only),
  // one for UI rendering (includes virtual Slack config fields).
  const { adapterVariableDefs, adapterDisplayFields } = useMemo(() => {
    const varDefs: Record<string, [string, VariableDisplay][]> = {};
    const displayDefs: Record<string, [string, VariableDisplay][]> = {};

    for (const adapter of AVAILABLE_ADAPTERS) {
      const hardcoded = adapterFields(adapter.id);
      if (template?.variables) {
        const derived = Object.entries(template.variables).filter(([, v]) =>
          v.targets.some((t) => t === `interface.${adapter.id}`),
        );
        if (derived.length > 0) {
          if (adapter.id === "slack" && derived.some(([key]) => key === SLACK_CONFIG_KEY)) {
            const enriched = derived
              .filter(([key]) => key !== SLACK_CONFIG_KEY)
              .map(([key, v]) => {
                const meta = hardcoded.find((c) => c.key === key);
                const display = toVariableDisplay(v);
                return [key, {
                  ...display,
                  description: meta?.description ?? display.description,
                  secret: display.secret ?? meta?.secret,
                  label: meta?.label,
                  placeholder: meta?.placeholder,
                  helpUrl: meta?.helpUrl,
                }] as [string, VariableDisplay];
              });
            const virtualConfig: [string, VariableDisplay][] = (ADAPTER_CONFIG.slack ?? []).map((c) => [
              c.key,
              { description: c.description, optional: true, secret: false, label: c.label, placeholder: c.placeholder },
            ]);
            varDefs[adapter.id] = enriched;
            displayDefs[adapter.id] = [...enriched, ...virtualConfig];
            continue;
          }

          const entries: [string, VariableDisplay][] = derived.map(([key, v]) => {
            const meta = hardcoded.find((c) => c.key === key);
            const display = toVariableDisplay(v);
            return [key, {
              ...display,
              description: meta?.description ?? display.description,
              secret: display.secret ?? meta?.secret,
              label: meta?.label,
              placeholder: meta?.placeholder,
              helpUrl: meta?.helpUrl,
            }];
          });
          varDefs[adapter.id] = entries;
          displayDefs[adapter.id] = entries;
          continue;
        }
      }
      const fallback: [string, VariableDisplay][] = hardcoded.map((c) => [c.key, {
        description: c.description,
        optional: c.optional ?? false,
        secret: c.secret,
        label: c.label,
        placeholder: c.placeholder,
        helpUrl: c.helpUrl,
      }]);
      varDefs[adapter.id] = fallback;
      displayDefs[adapter.id] = fallback;
    }
    return { adapterVariableDefs: varDefs, adapterDisplayFields: displayDefs };
  }, [template]);

  // Initialize variable values when template loads
  useEffect(() => {
    if (variableEntries.length > 0) {
      setVariableValues((prev) => {
        const initial: Record<string, string> = {};
        for (const [key, v] of variableEntries) {
          initial[key] = prev[key] ?? v.defaultValue ?? getVariableDefault(v);
        }
        return initial;
      });
    }
  }, [variableEntries]);

  // Seed adapter field defaults from the template so spec-defined values
  // (e.g. actionable_reactions: [ticket]) appear pre-filled on fresh install.
  useEffect(() => {
    const defaults: Record<string, string> = {};
    for (const defs of Object.values(adapterDisplayFields)) {
      for (const [key, v] of defs) {
        if (v.defaultValue) defaults[key] = v.defaultValue;
      }
    }
    // SLACK_CONFIG is a compound field — parse its default into the three virtual fields
    const slackCfgDefault = template?.variables?.[SLACK_CONFIG_KEY]?.default;
    if (slackCfgDefault) {
      const parsed = deserializeSlackConfig(slackCfgDefault);
      for (const [key, val] of Object.entries(parsed)) {
        if (val && !defaults[key]) defaults[key] = val;
      }
    }
    if (Object.keys(defaults).length > 0) {
      setAdapterCredentials((prev) => ({ ...defaults, ...prev }));
    }
  }, [adapterDisplayFields, template]);

  // Compute validation errors (only surfaced after first submit attempt)
  const errors = useMemo<FormErrors>(() => {
    if (!submitted) return {};

    const result: FormErrors = {};

    if (!targetAccount) {
      result.account = "Select an account to install under";
    }

    if (!deployName.trim()) {
      result.deployName = "Enter a name for the agent";
    } else if (deployName.trim().length > 64) {
      result.deployName = "Name must be 64 characters or fewer";
    }

    if (selectedAdapters.length === 0) {
      result.adapters = "Select at least one messaging type";
    }

    const emptyRequired = requiredVariables
      .filter(([key, v]) => !isVariableFilled(v, allFormValues[key]))
      .map(([key]) => key);
    if (emptyRequired.length > 0) {
      result.credentials = emptyRequired;
    }

    const emptyAdapterCreds = selectedAdapters.flatMap((adapterId) => {
      const creds = adapterDisplayFields[adapterId] ?? [];
      return creds
        .filter(([key, def]) => !def.optional && !allFormValues[key]?.trim())
        .map(([key]) => key);
    });
    if (emptyAdapterCreds.length > 0) {
      result.adapterCredentials = emptyAdapterCreds;
    }

    const emptySchedules = scheduleIngestions.filter(
      (name) => !ingestionSchedules[name]?.trim(),
    );
    if (emptySchedules.length > 0) {
      result.ingestionSchedules = emptySchedules;
    }

    return result;
  }, [submitted, targetAccount, deployName, selectedAdapters, requiredVariables, allFormValues, adapterDisplayFields, scheduleIngestions, ingestionSchedules]);

  const isValid = submitted
    ? !errors.account && !errors.deployName && !errors.adapters && !errors.credentials && !errors.adapterCredentials && !errors.ingestionSchedules
    : true;

  // Try to submit: marks form as submitted and returns validity
  const trySubmit = (): boolean => {
    setSubmitted(true);

    // Compute validity inline (state update is async, can't rely on `errors` yet)
    const hasAccount = !!targetAccount;
    const hasName = !!deployName.trim();
    const hasAdapter = selectedAdapters.length > 0;
    const varsValid = requiredVariables.every(([key, v]) => isVariableFilled(v, allFormValues[key]));
    const adapterCredsValid = selectedAdapters.every((adapterId) => {
      const creds = adapterDisplayFields[adapterId] ?? [];
      return creds.every(([key, def]) => def.optional || allFormValues[key]?.trim());
    });
    const schedulesValid = scheduleIngestions.every((n) => ingestionSchedules[n]?.trim());

    return hasAccount && hasName && hasAdapter && varsValid && adapterCredsValid && schedulesValid;
  };

  // Submission
  const deploy = async (): Promise<DeployResponse | undefined> => {
    if (!template || !account || !name) return;

    setDeployError(null);
    const spec = fulfillTemplate(
      template,
      allFormValues,
      selectedAdapters,
      adapterVariableDefs,
      targetAccount,
      deployName.trim(),
      ingestionSchedules,
    );

    try {
      return await deployMutation.mutateAsync(spec);
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

    accounts,
    targetAccount,
    setTargetAccount,

    deployName,
    setDeployName,

    selectedAdapters,
    setSelectedAdapters,
    adapterDisplayFields,
    adapterCredentials,
    setAdapterCredentials,

    variableValues,
    setVariableValues,
    requiredVariables,
    optionalVariables,

    scheduleIngestions,
    ingestionSchedules,
    setIngestionSchedules,

    errors,
    submitted,
    isValid,
    trySubmit,
    deploy,
    isDeploying: deployMutation.isPending,
    deployError,

    /**
     * Bulk-import parsed key-value pairs into the correct form state.
     * Keys matching template variables go into variableValues; keys matching
     * adapter credentials go into adapterCredentials; the rest are skipped.
     * Returns the list of matched and skipped keys for UI feedback.
     */
    bulkSetVariables(imported: Record<string, string>): { matched: string[]; skipped: string[] } {
      const variableKeys = new Set(variableEntries.map(([k]) => k));
      const adapterKeys = new Set(
        Object.values(adapterDisplayFields).flatMap((defs) => defs.map(([k]) => k)),
      );

      const matched: string[] = [];
      const skipped: string[] = [];
      const newVarValues: Record<string, string> = {};
      const newAdapterValues: Record<string, string> = {};

      for (const [key, value] of Object.entries(imported)) {
        const inVariableKeys = variableKeys.has(key);
        const inAdapterKeys = adapterKeys.has(key);
        if (inVariableKeys) {
          newVarValues[key] = value;
        }
        if (inAdapterKeys) {
          newAdapterValues[key] = value;
        }
        if (inVariableKeys || inAdapterKeys) {
          matched.push(key);
        } else {
          skipped.push(key);
        }
      }

      if (Object.keys(newVarValues).length > 0) {
        setVariableValues((prev) => ({ ...prev, ...newVarValues }));
      }
      if (Object.keys(newAdapterValues).length > 0) {
        setAdapterCredentials((prev) => ({ ...prev, ...newAdapterValues }));
      }

      return { matched, skipped };
    },

    reset(values?: DeployFormInitialValues) {
      const v = values ?? iv;
      setDeployName(v?.deployName ?? slugToTitle(name));
      setVariableValues(v?.variableValues ?? {});
      setSelectedAdapters(v?.selectedAdapters ?? ["web"]);
      setAdapterCredentials(v?.adapterCredentials ?? {});
      setIngestionSchedules(v?.ingestionSchedules ?? {});
    },
  };
}
