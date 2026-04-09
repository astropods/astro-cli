import { useState, useEffect, useMemo } from "react";
import { sentenceCase } from "change-case";
import type { ReactNode } from "react";
import { useDeploymentTemplate, useDeployAgent } from "@/api/queries/blueprints";
import { useAuth } from "@/lib/auth";
import type { DeploymentTemplate, DeploymentVariable, DeploymentSpec, ApiError } from "@/lib/api";
import type { VariableDisplay } from "./VariableFields";
import { getVariableDefault, isVariableFilled } from "./VariableField";
import { parseVaultToken } from "./VaultPicker";
import { useAccountVariables } from "@/api/queries";
import { SLACK_CONFIG_KEY, serializeSlackConfig, deserializeSlackConfig } from "./slackConfig";

function resolveValue(raw: string): Pick<DeploymentVariable, 'value' | 'ref'> {
  const parsed = parseVaultToken(raw);
  return parsed ? { ref: parsed.name } : { value: raw };
}

function isInvalidVaultRef(value: string, knownNames: Set<string>): boolean {
  const parsed = parseVaultToken(value);
  return parsed !== null && !knownNames.has(parsed.name);
}

export interface DeployFormInitialValues {
  deployName?: string;
  variableValues?: Record<string, string>;
  selectedAdapters?: string[];
  adapterCredentials?: Record<string, string>;
  targetAccount?: string;
  ingestionSchedules?: Record<string, string>;
  webAuthEnabled?: boolean;
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
  icon?: string;
  datatype?: string;
  defaultValue?: string;
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
    label: v.label,
    icon: v.icon,
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

/**
 * Builds the interfaces payload for a deployment spec.
 * When the web adapter is selected, forces expose.enabled=true on the HTTP
 * endpoint so the chat UI gets an ingress and is publicly accessible.
 */
export function buildInterfacesPayload(
  interfaces: Record<string, unknown>,
  selectedAdapters: string[],
  webAuthEnabled: boolean,
): Record<string, unknown> {
  const endpoints = selectedAdapters.includes("web") && interfaces.endpoints
    ? {
        ...(interfaces.endpoints as Record<string, unknown>),
        http: {
          ...((interfaces.endpoints as Record<string, unknown>).http as Record<string, unknown>),
          expose: { enabled: true },
        },
      }
    : interfaces.endpoints;
  return {
    ...interfaces,
    adapters: selectedAdapters,
    endpoints,
    auth: webAuthEnabled ? { web: { type: "oidc" } } : undefined,
  };
}

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
  webAuthEnabled: boolean,
): DeploymentSpec {
  // Destructure out editable (template-only) so it is not present in the fulfilled spec
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- intentionally omitting editable from rest
  const { editable: _editable, ...rest } = template;

  // Rebuild variables: keep only runtime fields, fill in user-supplied value or vault ref
  const variables: Record<string, DeploymentVariable> = rest.variables
    ? Object.fromEntries(
        Object.entries(rest.variables).map(([key, { targets, secret, optional }]) => [
          key,
          { ...resolveValue(variableValues[key] ?? ''), targets, secret, optional },
        ]),
      )
    : {};
  // Inject adapter credentials not already declared in template variables
  for (const adapterId of selectedAdapters) {
    const creds = adapterVariableDefs[adapterId] ?? [];
    for (const [key, cred] of creds) {
      if (!(key in variables)) {
        variables[key] = {
          ...resolveValue(variableValues[key] ?? ''),
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
      ? buildInterfacesPayload(rest.interfaces, selectedAdapters, webAuthEnabled)
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

  // Derive targetAccount reactively: explicit initialValue > user selection > personalAccount.
  // Do NOT initialize from personalAccount directly in useState — if the hook
  // mounts before auth resolves, personalAccount is null and the value freezes as "".
  const [_targetAccount, setTargetAccount] = useState(iv?.targetAccount ?? "");
  const targetAccount = _targetAccount || personalAccount?.name || "";
  const { data: accountVarsData, isSuccess: accountVarsLoaded } = useAccountVariables(targetAccount);
  const accountVarNames = useMemo(
    () => new Set(accountVarsData?.variables.map(v => v.name) ?? []),
    [accountVarsData?.variables],
  );

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
  const [webAuthEnabled, setWebAuthEnabled] = useState<boolean>(iv?.webAuthEnabled ?? false);
  const [ingestionSchedules, setIngestionSchedules] = useState<Record<string, string>>(iv?.ingestionSchedules ?? {});
  const [deployError, setDeployError] = useState<{ message: string; details?: string } | null>(null);
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
                  icon: meta?.icon,
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
              defaultValue: display.defaultValue ?? meta?.defaultValue,
              datatype: meta?.datatype ?? display.datatype,
              secret: display.secret ?? meta?.secret,
              label: meta?.label,
              icon: meta?.icon,
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
        defaultValue: c.defaultValue,
        datatype: c.datatype,
        optional: c.optional ?? false,
        secret: c.secret,
        label: c.label,
        icon: c.icon,
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

  // Always-on vault ref validation — not gated by submitted so chips turn red
  // immediately when the target account changes, without requiring a submit attempt.
  const invalidVaultRefKeys = useMemo(
    () => accountVarsLoaded
      ? variableEntries
          .filter(([key]) => isInvalidVaultRef(allFormValues[key] ?? '', accountVarNames))
          .map(([key]) => key)
      : [],
    [accountVarsLoaded, variableEntries, allFormValues, accountVarNames],
  );

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

    const credErrors = [...new Set([...emptyRequired, ...invalidVaultRefKeys])];
    if (credErrors.length > 0) {
      result.credentials = credErrors;
    }

    const emptyAdapterCreds = selectedAdapters.flatMap((adapterId) => {
      const creds = adapterDisplayFields[adapterId] ?? [];
      return creds
        .filter(([key, def]) => !def.optional && !isVariableFilled(def, allFormValues[key]))
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
  }, [submitted, targetAccount, deployName, selectedAdapters, requiredVariables, allFormValues, adapterDisplayFields, scheduleIngestions, ingestionSchedules, invalidVaultRefKeys]);

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
      return creds.every(([key, def]) => def.optional || isVariableFilled(def, allFormValues[key]));
    });
    const schedulesValid = scheduleIngestions.every((n) => ingestionSchedules[n]?.trim());
    const vaultRefsValid = invalidVaultRefKeys.length === 0;

    return hasAccount && hasName && hasAdapter && varsValid && adapterCredsValid && schedulesValid && vaultRefsValid;
  };

  // Submission
  const deploy = async () => {
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
      webAuthEnabled,
    );

    try {
      return await deployMutation.mutateAsync(spec);
    } catch (err) {
      const apiErr = err as ApiError;
      const details: string[] = [];
      if (apiErr.validation_errors?.length) {
        for (const ve of apiErr.validation_errors) details.push(`${ve.field}: ${ve.message}`);
      }
      if (apiErr.missing_variables?.length) {
        details.push(`Missing variables: ${apiErr.missing_variables.join(", ")}`);
      }
      const rawMessage = apiErr.error ?? (err instanceof Error ? err.message : "Deployment failed");
      const message = sentenceCase(rawMessage);
      const detailText = details.length > 0
        ? details.join("\n")
        : typeof apiErr.details === "string" ? apiErr.details : undefined;
      setDeployError({ message, details: detailText });
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
    webAuthEnabled,
    setWebAuthEnabled,

    variableValues,
    setVariableValues,
    requiredVariables,
    optionalVariables,

    scheduleIngestions,
    ingestionSchedules,
    setIngestionSchedules,

    vaultEntries: accountVarsData?.variables ?? [],
    vaultSettingsUrl: (() => {
      const acct = accounts.find(a => a.name === targetAccount);
      if (!acct || acct.type === 'personal') return '/settings/secrets';
      return `/settings/org/${targetAccount}/secrets`;
    })(),

    errors,
    invalidVaultRefKeys,
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
      setWebAuthEnabled(v?.webAuthEnabled ?? false);
    },
  };
}
