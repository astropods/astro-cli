import { useState, useEffect, useMemo, useDeferredValue, useRef, useCallback } from "react";
import { sentenceCase } from "change-case";
import type { ReactNode } from "react";
import { usePostDeploymentTemplate, useDeployAgent } from "@/api/queries/blueprints";
import { useAuth } from "@/lib/auth";
import type { DeploymentTemplate, DeploymentVariable, DeploymentSpec, ApiError, TemplateResponse, TemplateRequest, TemplateInterfaces } from "@/lib/api";
import { ApiRequestError } from "@/lib/api";
import type { VariableDisplay } from "./VariableFields";
import { getVariableDefault, isVariableFilled } from "./VariableField";
import { parseVaultToken } from "./VaultPicker";
import { useAccountVariables } from "@/api/queries";
import { serializeObjectVariable, deserializeObjectVariable } from "./slackConfig";
import { computeFormDefaults } from "./computeFormDefaults";

function resolveValue(raw: string): Pick<DeploymentVariable, 'value' | 'ref'> {
  const parsed = parseVaultToken(raw);
  return parsed ? { ref: parsed.name } : { value: raw };
}

function isWebAuthOidc(auth: TemplateInterfaces['auth'] | undefined): boolean {
  return auth?.web?.type === "oidc";
}

function isInvalidVaultRef(value: string, knownNames: Set<string>): boolean {
  const parsed = parseVaultToken(value);
  return parsed !== null && !knownNames.has(parsed.name);
}

function formatVaultVariablesLoadError(err: unknown): string {
  if (err instanceof ApiRequestError) return err.message;
  if (err instanceof Error) return err.message;
  return "Could not load variables for this account.";
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
  /** SSR-prefetched template response (POST format). */
  initialTemplateResponse?: TemplateResponse;
  /** Pre-fill form state (e.g. from an existing deployment's spec). */
  initialValues?: DeployFormInitialValues;
  /** Restrict which accounts can be selected as deployment targets. */
  allowedTargetAccounts?: string[];
  /** Existing deployment ID for prefill (redeploy/configure). */
  deploymentId?: string;
  /** Pin to a specific build ID (e.g. for new-build upgrades). */
  build?: string;
  /** Load a historical revision's config (requires deploymentId). */
  revision?: number;
}

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

/** Check whether a variable is an object with sub-field schema. */
function isObjectVariable(v: DeploymentVariable): boolean {
  return v.datatype === "object" && !!v.fields && Object.keys(v.fields).length > 0;
}

/** Compute form-ready initial values from a pre-filled deployment template.
 *  @param respInterfaces — top-level `interfaces` from TemplateResponse (adapters + auth)
 *  @param respSchedules — top-level `schedules` from TemplateResponse (ingestion name → cron) */
export function computeInitialValues(template: DeploymentTemplate, account: string, respInterfaces?: TemplateInterfaces, respSchedules?: Record<string, string>): DeployFormInitialValues {
  const variableValues: Record<string, string> = {};
  const adapterCredentials: Record<string, string> = {};

  if (template.variables) {
    for (const [key, v] of Object.entries(template.variables)) {
      const val = v.ref
        ? (v.secret ? `{{secrets.${v.ref}}}` : `{{vars.${v.ref}}}`)
        : (v.value ?? v.default ?? getVariableDefault({ datatype: v.datatype }));
      // Object variables with fields → expand sub-fields into adapter credentials
      if (isObjectVariable(v)) {
        const parsed = deserializeObjectVariable(key, v.fields!, val);
        for (const [fKey, fVal] of Object.entries(parsed)) {
          adapterCredentials[fKey] = fVal;
        }
        continue;
      }
      if (v.targets?.some((t: string) => t.startsWith("interface."))) {
        adapterCredentials[key] = val;
      } else {
        variableValues[key] = val;
      }
    }
  }

  const adapters = respInterfaces?.adapters;
  const selectedAdapters: string[] = Array.isArray(adapters) && adapters.length > 0 ? adapters : ["web"];
  const webAuthEnabled = isWebAuthOidc(respInterfaces?.auth);

  const ingestionSchedules: Record<string, string> = respSchedules ?? {};
  if (!respSchedules && template.ingestion) {
    for (const [name, ing] of Object.entries(template.ingestion)) {
      if (ing.trigger?.type === "schedule") {
        ingestionSchedules[name] = ing.trigger.schedule ?? "";
      }
    }
  }

  return {
    deployName: template.target.display_name || "",
    targetAccount: account,
    variableValues,
    selectedAdapters,
    adapterCredentials,
    ingestionSchedules,
    webAuthEnabled,
  };
}

function toVariableDisplay(v: DeploymentVariable): VariableDisplay {
  return {
    description: v.description,
    optional: v.optional,
    secret: v.secret,
    label: v.label,
    placeholder: v.placeholder,
    helpUrl: v.help_url,
    icon: v.icon,
    datatype: v.datatype,
    displayAs: v['display-as'],
    options: v.options,
    defaultValue: v.default,
  };
}

/**
 * Converts a POST TemplateResponse into the legacy DeploymentTemplate shape
 * so existing form logic (variable rendering, adapter fields) works unchanged.
 */
function toDeploymentTemplate(resp: TemplateResponse): DeploymentTemplate {
  return {
    ...resp.template,
    spec: 'deployment-template/v1',
    variables: resp.variables,
    editable: resp.editable,
  } as DeploymentTemplate;
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
  const allowedTargetAccounts = opts?.allowedTargetAccounts;
  const selectableAccounts = useMemo(() => {
    if (!allowedTargetAccounts?.length) return accounts;
    const allowed = new Set(allowedTargetAccounts);
    return accounts.filter((acct) => allowed.has(acct.name));
  }, [accounts, allowedTargetAccounts]);

  // Derive targetAccount reactively: explicit initialValue > user selection > personalAccount.
  // Do NOT initialize from personalAccount directly in useState — if the hook
  // mounts before auth resolves, personalAccount is null and the value freezes as "".
  const [_targetAccount, setTargetAccount] = useState(iv?.targetAccount ?? "");
  const rawTargetAccount = _targetAccount || personalAccount?.name || "";
  const targetAccount = (() => {
    if (!allowedTargetAccounts?.length) return rawTargetAccount;
    if (rawTargetAccount && allowedTargetAccounts.includes(rawTargetAccount)) return rawTargetAccount;
    return selectableAccounts[0]?.name ?? "";
  })();
  const setAllowedTargetAccount = useCallback((next: string) => {
    if (allowedTargetAccounts?.length && !allowedTargetAccounts.includes(next)) return;
    setTargetAccount(next);
  }, [allowedTargetAccounts]);
  const {
    data: accountVarsData,
    isSuccess: accountVarsLoaded,
    isError: vaultVarsQueryFailed,
    error: vaultVarsQueryError,
  } = useAccountVariables(targetAccount);
  const vaultEntriesLoadError = vaultVarsQueryFailed
    ? formatVaultVariablesLoadError(vaultVarsQueryError)
    : null;
  const accountVarNames = useMemo(
    () => new Set(accountVarsData?.variables.map(v => v.name) ?? []),
    [accountVarsData?.variables],
  );

  const [initialValues, setInitialValues] = useState<DeployFormInitialValues | null>(null);
  const seededRef = useRef(false);

  // Fetch template via interactive POST endpoint.
  const templateMutation = usePostDeploymentTemplate(account, name);
  const [templateResponse, setTemplateResponse] = useState<TemplateResponse | null>(
    opts?.initialTemplateResponse ?? null,
  );
  const fetchedForRef = useRef<string | null>(
    opts?.initialTemplateResponse ? `${account}/${name}` : null,
  );
  const [fetchError, setFetchError] = useState<Error | null>(null);

  useEffect(() => {
    const key = `${account}/${name}`;
    if (fetchedForRef.current === key) return;
    if (!account || !name) return;
    fetchedForRef.current = key;
    seededRef.current = false;
    setTemplateResponse(null);
    setFetchError(null);
    const body: TemplateRequest = {};
    if (opts?.deploymentId) body.deployment_id = opts.deploymentId;
    if (opts?.build) body.build = opts.build;
    if (opts?.revision) body.revision = opts.revision;
    templateMutation.mutateAsync(body).then(setTemplateResponse, setFetchError);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- re-fetch only when agent identity changes
  }, [account, name]);

  const templateLoading = !templateResponse && !fetchError;
  const templateError = fetchError;

  // Derive legacy DeploymentTemplate shape for existing form logic.
  const template: DeploymentTemplate | null = useMemo(() => {
    if (templateResponse) return toDeploymentTemplate(templateResponse);
    return null;
  }, [templateResponse]);

  const deployMutation = useDeployAgent(targetAccount, name);

  // Compute initial form state synchronously so the form is correct on the
  // first render. When initialValues are provided (settings page), use those.
  // Otherwise, derive defaults from the template (fresh deploy page).
  // The POST-based seeding effect will override these once the template loads.
  // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally computed once at mount
  const computedDefaults = useMemo(() => iv ?? computeFormDefaults(null, name), []);

  const [deployName, setDeployName] = useState(() => computedDefaults.deployName ?? slugToTitle(name));
  const [variableValues, setVariableValues] = useState<Record<string, string>>(computedDefaults.variableValues ?? {});
  const [selectedAdapters, setSelectedAdaptersRaw] = useState<string[]>(computedDefaults.selectedAdapters ?? ["web"]);
  const [adapterCredentials, setAdapterCredentials] = useState<Record<string, string>>(computedDefaults.adapterCredentials ?? {});
  const [webAuthEnabled, setWebAuthEnabled] = useState<boolean>(computedDefaults.webAuthEnabled ?? false);
  const [ingestionSchedules, setIngestionSchedules] = useState<Record<string, string>>(computedDefaults.ingestionSchedules ?? {});
  const [knowledgeBindings, setKnowledgeBindingsRaw] = useState<Record<string, string>>({});
  const [deployError, setDeployError] = useState<{ message: string; details?: string } | null>(null);
  const [submitted, setSubmitted] = useState(false);

  // Applies a set of form values to all state variables at once.
  // Used by both the initial seeding effect and `reset()`.
  //
  // targetAccount is part of the seeded set: extracted.targetAccount comes
  // from the URL/owning account, and silently dropping it caused redeploys
  // from the configure page to fall back to personalAccount.name, which the
  // server rejects as a cross-account private deploy ("source agent not
  // found") whenever the URL account differs from the user's personal one.
  const applyValues = (v: DeployFormInitialValues) => {
    // deployName uses || because "" should fall through to the slugToTitle fallback
    setDeployName(v.deployName || slugToTitle(name));
    setVariableValues(v.variableValues ?? {});
    setSelectedAdaptersRaw(v.selectedAdapters ?? ["web"]);
    setAdapterCredentials(v.adapterCredentials ?? {});
    setIngestionSchedules(v.ingestionSchedules ?? {});
    setWebAuthEnabled(v.webAuthEnabled ?? false);
    if (v.targetAccount !== undefined) {
      setTargetAccount(v.targetAccount);
    }
  };

  // Seed all form state from the template in one pass once it loads.
  // Uses v.value (existing deployment values) not just v.default, so both
  // fresh deploys and configure pages work correctly without manual seeding.
  useEffect(() => {
    if (!template || seededRef.current) return;
    seededRef.current = true;

    const extracted = computeInitialValues(template, account, templateResponse?.interfaces, templateResponse?.schedules);

    // Seed knowledge bindings from prefilled template response.
    if (templateResponse?.bindings?.knowledge) {
      const prefilled: Record<string, string> = {};
      for (const [entryName, info] of Object.entries(templateResponse.bindings.knowledge)) {
        if (info.arn) prefilled[entryName] = info.arn;
      }
      if (Object.keys(prefilled).length > 0) {
        setKnowledgeBindingsRaw(prefilled);
      }
    }
    const merged: DeployFormInitialValues = {
      deployName: iv?.deployName || extracted.deployName || slugToTitle(name),
      targetAccount: iv?.targetAccount ?? extracted.targetAccount ?? "",
      variableValues: { ...extracted.variableValues, ...(iv?.variableValues ?? {}) },
      selectedAdapters: iv?.selectedAdapters ?? extracted.selectedAdapters ?? ["web"],
      adapterCredentials: { ...extracted.adapterCredentials, ...(iv?.adapterCredentials ?? {}) },
      ingestionSchedules: { ...extracted.ingestionSchedules, ...(iv?.ingestionSchedules ?? {}) },
      webAuthEnabled: iv?.webAuthEnabled ?? extracted.webAuthEnabled ?? false,
    };

    setInitialValues(merged);
    applyValues(merged);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- seed once when template first loads
  }, [template]);

  // Re-POST template with new inputs to reshape variable optionality etc.
  // Does NOT reset form values — only updates the template schema.
  const reshapeTemplate = useCallback((inputs: TemplateRequest) => {
    const body: TemplateRequest = { ...inputs };
    if (opts?.deploymentId) body.deployment_id = opts.deploymentId;
    if (opts?.build) body.build = opts.build;
    templateMutation.mutateAsync(body).then(setTemplateResponse, setFetchError);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- stable identity for opts
  }, [opts?.deploymentId, opts?.build]);

  // Build the interfaces payload from current form state.
  const buildInterfaces = useCallback((): TemplateInterfaces => ({
    adapters: selectedAdapters,
    auth: webAuthEnabled ? { web: { type: "oidc" } } : undefined,
  }), [selectedAdapters, webAuthEnabled]);

  // Exposed adapter setter: updates local state AND re-triggers template shaping
  // so the server can flip variable optionality (e.g. Slack tokens become required).
  // bindings.knowledge is always emitted (even when empty) — see setKnowledgeBindings for why.
  const setSelectedAdapters = useCallback((adapters: string[]) => {
    setSelectedAdaptersRaw(adapters);
    reshapeTemplate({
      interfaces: { adapters, auth: webAuthEnabled ? { web: { type: "oidc" } } : undefined },
      bindings: { knowledge: nonEmptyBindings(knowledgeBindings) },
    });
  }, [reshapeTemplate, webAuthEnabled, knowledgeBindings]);

  // Filter empty-string ARNs — an entry with value "" means "not bound".
  const nonEmptyBindings = (b: Record<string, string>): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(b)) {
      if (v) out[k] = v;
    }
    return out;
  };

  // Exposed binding setter: updates state and re-POSTs to reshape the template.
  // Binding selection is a structural change (removes/adds knowledge entries, variables, editable fields).
  // Always send a (possibly empty) knowledge map so the server preserves
  // explicit clears — sending `bindings: undefined` would let the server
  // restore stored bindings on top of a user who cleared all of them.
  const setKnowledgeBindings = useCallback((bindings: Record<string, string>) => {
    const cleaned = nonEmptyBindings(bindings);
    setKnowledgeBindingsRaw(cleaned);
    reshapeTemplate({
      interfaces: buildInterfaces(),
      bindings: { knowledge: cleaned },
    });
  }, [reshapeTemplate, buildInterfaces]);

  const allFormValues = useMemo(
    () => mergeFormValues(variableValues, adapterCredentials),
    [variableValues, adapterCredentials],
  );

  const scheduleIngestions = useMemo<string[]>(
    () =>
      templateResponse?.schedules
        ? Object.keys(templateResponse.schedules).sort()
        : template?.ingestion
          ? Object.entries(template.ingestion)
              .filter(([, ing]) => ing.trigger?.type === "schedule")
              .map(([name]) => name)
          : [],
    [template, templateResponse],
  );

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

  // Derive adapter fields from the server template — grouped by interface target.
  // Two views: varDefs for submission (real template variables), displayDefs for
  // UI rendering (includes virtual Slack config fields parsed from SLACK_CONFIG).
  const { adapterVariableDefs, adapterDisplayFields } = useMemo(() => {
    const varDefs: Record<string, [string, VariableDisplay][]> = {};
    const displayDefs: Record<string, [string, VariableDisplay][]> = {};

    if (!template?.variables) return { adapterVariableDefs: varDefs, adapterDisplayFields: displayDefs };

    // Group variables by adapter target (extract adapter name from "interface.{name}")
    const byAdapter = new Map<string, [string, DeploymentVariable][]>();
    for (const [key, v] of Object.entries(template.variables)) {
      for (const t of v.targets ?? []) {
        const match = t.match(/^interface\.(.+)$/);
        if (match) {
          const adapterId = match[1];
          if (!byAdapter.has(adapterId)) byAdapter.set(adapterId, []);
          byAdapter.get(adapterId)!.push([key, v]);
        }
      }
    }

    for (const [adapterId, vars] of byAdapter) {
      const isSelected = selectedAdapters.includes(adapterId);

      // Separate object variables (expanded into sub-fields) from regular variables.
      const objectVars = vars.filter(([, v]) => isObjectVariable(v));
      const realFields: [string, VariableDisplay][] = vars
        .filter(([, v]) => !isObjectVariable(v))
        .map(([key, v]) => {
          const display = toVariableDisplay(v);
          // When the adapter is selected, secret token variables become required immediately
          // (optimistic — confirmed by the server reshape response).
          if (isSelected && display.secret) display.optional = false;
          return [key, display];
        });

      varDefs[adapterId] = realFields;

      // Expand object variables into per-sub-field display entries driven by the server schema.
      const subFields: [string, VariableDisplay][] = [];
      for (const [parentKey, v] of objectVars) {
        for (const [fieldKey, fieldDef] of Object.entries(v.fields!)) {
          subFields.push([`${parentKey}.${fieldKey}`, {
            label: fieldDef.label,
            description: fieldDef.description,
            placeholder: fieldDef.placeholder,
            optional: fieldDef.optional ?? true,
            secret: false,
          }]);
        }
      }
      displayDefs[adapterId] = subFields.length > 0
        ? [...realFields, ...subFields]
        : realFields;
    }

    return { adapterVariableDefs: varDefs, adapterDisplayFields: displayDefs };
  }, [template, selectedAdapters]);

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

  // Submission: POST template with all inputs, then deploy with the fulfilled spec.
  const deploy = async () => {
    if (!template || !account || !name) {
      console.warn('[useDeployForm] deploy() called before template loaded');
      return;
    }

    setDeployError(null);

    // Build the variables payload from form values.
    const variableInputs: Record<string, { value?: string; ref?: string }> = {};
    for (const [key] of [...variableEntries, ...Object.values(adapterVariableDefs).flat()]) {
      const raw = allFormValues[key];
      if (raw != null && raw !== "") {
        variableInputs[key] = resolveValue(raw);
      }
    }
    // Serialize object variables: re-assemble sub-field form values into JSON.
    for (const [key, v] of Object.entries(template.variables ?? {})) {
      if (isObjectVariable(v)) {
        variableInputs[key] = { value: serializeObjectVariable(key, v.fields!, allFormValues) };
      }
    }

    // POST template with all inputs to get the server-fulfilled spec.
    // bindings.knowledge is always emitted (even when empty) — see setKnowledgeBindings for why.
    const req: TemplateRequest = {
      interfaces: buildInterfaces(),
      variables: variableInputs,
      schedules: ingestionSchedules,
      bindings: { knowledge: nonEmptyBindings(knowledgeBindings) },
    };
    if (opts?.deploymentId) req.deployment_id = opts.deploymentId;
    if (opts?.build) req.build = opts.build;
    req.finalize = true;

    let resp: TemplateResponse;
    try {
      resp = await templateMutation.mutateAsync(req);
    } catch (err) {
      const apiErr = err as ApiError;
      setDeployError({
        message: sentenceCase(apiErr.error ?? "Failed to validate deployment"),
        details: apiErr.details as string | undefined,
      });
      throw err;
    }

    if (!resp.validation.valid) {
      const messages = resp.validation.errors.map(e => `${e.field}: ${e.message}`);
      setDeployError({ message: "Validation failed", details: messages.join("\n") });
      return;
    }

    // The server-fulfilled template has schedules baked in — just patch target.
    const spec: DeploymentSpec = {
      ...resp.template,
      target: { ...resp.template.target, account: targetAccount, display_name: deployName.trim() },
    };

    try {
      return await deployMutation.mutateAsync({ spec, signature: resp.signature });
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

  // Dirty detection — compare current state against initial values
  const { nameChanged, deployChanged, isDirty, changeCount } = useMemo(() => {
    if (!initialValues) return { nameChanged: false, deployChanged: false, isDirty: false, changeCount: 0 };

    const nameChanged = deployName !== (initialValues.deployName || slugToTitle(name));

    // Count individual field-level changes
    let deployCount = 0;

    // Variables — count each key that differs
    const ivVars = initialValues.variableValues ?? {};
    const allVarKeys = new Set([...Object.keys(variableValues), ...Object.keys(ivVars)]);
    for (const k of allVarKeys) {
      if ((variableValues[k] ?? "") !== (ivVars[k] ?? "")) deployCount++;
    }

    // Adapter credentials — count each key that differs
    const ivCreds = initialValues.adapterCredentials ?? {};
    const allCredKeys = new Set([...Object.keys(adapterCredentials), ...Object.keys(ivCreds)]);
    for (const k of allCredKeys) {
      if ((adapterCredentials[k] ?? "") !== (ivCreds[k] ?? "")) deployCount++;
    }

    // Adapters — count each added or removed
    const ivAdapters = initialValues.selectedAdapters ?? ["web"];
    const added = selectedAdapters.filter((a) => !ivAdapters.includes(a));
    const removed = ivAdapters.filter((a) => !selectedAdapters.includes(a));
    deployCount += added.length + removed.length;

    // Schedules — count each key that differs
    const ivSchedules = initialValues.ingestionSchedules ?? {};
    const allSchedKeys = new Set([...Object.keys(ingestionSchedules), ...Object.keys(ivSchedules)]);
    for (const k of allSchedKeys) {
      if ((ingestionSchedules[k] ?? "") !== (ivSchedules[k] ?? "")) deployCount++;
    }

    // Knowledge bindings — count each key that differs
    const allBindKeys = new Set(Object.keys(knowledgeBindings));
    for (const k of allBindKeys) {
      if (knowledgeBindings[k]) deployCount++;
    }

    // Web auth toggle
    if (webAuthEnabled !== (initialValues.webAuthEnabled ?? false)) deployCount++;

    const deployChanged = deployCount > 0;
    const changeCount = (nameChanged ? 1 : 0) + deployCount;

    return { nameChanged, deployChanged, isDirty: nameChanged || deployChanged, changeCount };
  }, [initialValues, deployName, name, variableValues, selectedAdapters, adapterCredentials, webAuthEnabled, ingestionSchedules, knowledgeBindings]);

  const deferredDirty = useDeferredValue({ nameChanged, deployChanged, isDirty, changeCount });

  return {
    template,
    templateLoading,
    templateErrorMessage,
    serverValidation: templateResponse?.validation ?? null,
    initialValues,

    nameChanged: deferredDirty.nameChanged,
    deployChanged: deferredDirty.deployChanged,
    isDirty: deferredDirty.isDirty,
    changeCount: deferredDirty.changeCount,

    accounts: selectableAccounts,
    targetAccount,
    setTargetAccount: setAllowedTargetAccount,

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

    knowledgeBindings,
    setKnowledgeBindings,
    resolvedBindings: templateResponse?.bindings?.knowledge ?? {},
    knowledgeEntries: template?.knowledge as Record<string, { provider?: string; binding?: string }> | undefined,

    vaultEntries: accountVarsData?.variables ?? [],
    vaultEntriesLoadError,
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

        // Object variables (e.g. SLACK_CONFIG): deserialize JSON into sub-field form keys
        const v = template?.variables?.[key];
        if (v && isObjectVariable(v)) {
          const subFields = deserializeObjectVariable(key, v.fields!, value);
          if (Object.keys(subFields).length > 0) {
            Object.assign(newAdapterValues, subFields);
            matched.push(key);
            continue;
          }
        }

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
      const resolved = values ?? initialValues ?? iv ?? computedDefaults;
      if (values) setInitialValues(values);
      applyValues(resolved);
    },
  };
}
