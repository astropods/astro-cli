import { useState, useEffect, useMemo, useDeferredValue, useRef, useCallback } from "react";
import { sentenceCase } from "change-case";
import type { ReactNode } from "react";
import { usePostDeploymentTemplate, useDeployAgent } from "@/api/queries/blueprints";
import { useAuth } from "@/lib/auth";
import type { DeploymentTemplate, DeploymentVariable, DeploymentSpec, ApiError, TemplateResponse, TemplateRequest, TemplateProvisioning, TemplateInterfaces, AuthGrant } from "@/lib/api";
import { ApiRequestError } from "@/lib/api";
import { accountSettingsPath } from "@/lib/settings-paths";
import type { VariableDisplay } from "./VariableFields";
import { getVariableDefault, isVariableFilled } from "./VariableField";
import { parseVaultToken } from "./VaultPicker";
import { useAccountVariables } from "@/api/queries";
import { serializeObjectVariable, deserializeObjectVariable } from "./slackConfig";
import { computeFormDefaults } from "./computeFormDefaults";
import { DEFAULT_AGENT_VOLUME_MOUNT, DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH } from "./constants";
import {
  knowledgeBindingChangeCount,
  provisioningChangeCount,
  type KnowledgeBindingMode,
  type KnowledgeBindingModes,
} from "./changeTracking";

function resolveValue(raw: string): Pick<DeploymentVariable, 'value' | 'ref'> {
  const parsed = parseVaultToken(raw);
  return parsed ? { ref: parsed.name } : { value: raw };
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
  webGrants?: AuthGrant[];
  slackGrants?: AuthGrant[];
  customPublic?: boolean;
  customGrants?: AuthGrant[];
  knowledgeBindings?: Record<string, string>;
  knowledgeBindingModes?: KnowledgeBindingModes;
  agentCpu?: string;
  agentMemory?: string;
  agentVolumeMount?: string;
  agentStorageSize?: string;
  agentResponseTimeout?: string;
}

interface ComputeInitialValuesOptions {
  preserveEmptyAdapters?: boolean;
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
  { id: "web", label: "Astro Chat", description: "Chat directly in the browser" },
];

/** Check whether a variable is an object with sub-field schema. */
function isObjectVariable(v: DeploymentVariable): boolean {
  return v.datatype === "object" && !!v.fields && Object.keys(v.fields).length > 0;
}

/** Messaging is supported when the interfaces block carries a messaging sidecar
 *  image. Keyed off the image — a stable agent capability — rather than the
 *  presence of an `interfaces` block or the current adapter list: a
 *  custom-interface-only agent gains an interfaces block (to hold auth.custom)
 *  but no image, and deselecting every adapter must not unmount the section. */
function agentHasMessaging(template: DeploymentTemplate | null): boolean {
  const iface = template?.interfaces as { image?: string } | undefined;
  return !!iface?.image;
}

/** The agent ships its own custom web interface when one of its endpoints is
 *  exposed. Distinct from the platform messaging-web adapter. */
function agentHasCustomInterface(template: DeploymentTemplate | null): boolean {
  const endpoints = (template?.agent as {
    endpoints?: Record<string, { expose?: { enabled?: boolean } }>;
  } | undefined)?.endpoints;
  if (!endpoints) return false;
  return Object.values(endpoints).some((ep) => ep?.expose?.enabled);
}

function knowledgeEntriesFromTemplate(
  template: DeploymentTemplate | null | undefined,
): Record<string, { provider?: string; binding?: string }> {
  return (template?.knowledge ?? {}) as Record<string, { provider?: string; binding?: string }>;
}

function bindingsFromKnowledgeEntries(
  entries: Record<string, { binding?: string }> | undefined,
): Record<string, string> {
  const bindings: Record<string, string> = {};
  if (!entries) return bindings;
  for (const [name, entry] of Object.entries(entries)) {
    if (entry.binding) bindings[name] = entry.binding;
  }
  return bindings;
}

function bindingsFromTemplateResponse(
  resp: TemplateResponse,
  template: DeploymentTemplate,
): Record<string, string> {
  const bindings = bindingsFromKnowledgeEntries(knowledgeEntriesFromTemplate(template));
  for (const [name, info] of Object.entries(resp.bindings?.knowledge ?? {})) {
    if (info.arn) bindings[name] = info.arn;
  }
  return bindings;
}

function knowledgeModesFromBindings(
  entries: Record<string, { binding?: string }> | undefined,
  bindings: Record<string, string>,
  explicitModes?: KnowledgeBindingModes,
): KnowledgeBindingModes {
  const modes: KnowledgeBindingModes = {};
  const names = new Set([
    ...Object.keys(entries ?? {}),
    ...Object.keys(bindings),
    ...Object.keys(explicitModes ?? {}),
  ]);
  for (const name of names) {
    modes[name] = bindings[name] ? "shared" : explicitModes?.[name] ?? "local";
  }
  return modes;
}

function sharedKnowledgeEntriesMissingBinding(
  entries: Record<string, { provider?: string; binding?: string }> | undefined,
  bindings: Record<string, string>,
  modes: KnowledgeBindingModes,
): string[] {
  if (!entries) return [];
  return Object.keys(entries)
    .filter((name) => (modes[name] ?? (bindings[name] ? "shared" : "local")) === "shared")
    .filter((name) => !bindings[name]?.trim())
    .sort();
}

/** Compute form-ready initial values from a pre-filled deployment template.
 *  @param respInterfaces — top-level `interfaces` from TemplateResponse (adapters + auth)
 *  @param respSchedules — top-level `schedules` from TemplateResponse (ingestion name → cron) */
export function computeInitialValues(
  template: DeploymentTemplate,
  account: string,
  respInterfaces?: TemplateInterfaces,
  respSchedules?: Record<string, string>,
  options: ComputeInitialValuesOptions = {},
): DeployFormInitialValues {
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

  // Messaging support is keyed off the sidecar image, not the mere presence of
  // an `interfaces` block: a custom-interface-only agent gains an interfaces
  // block (to carry auth.custom) but no messaging image, and must not default
  // to the "web" adapter.
  const hasMessaging = agentHasMessaging(template);
  const adapters = respInterfaces?.adapters;
  const selectedAdapters: string[] = Array.isArray(adapters) && (adapters.length > 0 || options.preserveEmptyAdapters)
    ? adapters
    : hasMessaging ? ["web"] : [];
  const webGrants = respInterfaces?.auth?.web?.grants ?? [];
  const slackGrants = respInterfaces?.auth?.slack?.grants ?? [];
  const customPublic = respInterfaces?.auth?.custom?.public ?? false;
  const customGrants = respInterfaces?.auth?.custom?.grants ?? [];
  const knowledgeEntries = knowledgeEntriesFromTemplate(template);
  const knowledgeBindings = bindingsFromKnowledgeEntries(knowledgeEntries);

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
    webGrants,
    slackGrants,
    customPublic,
    customGrants,
    knowledgeBindings,
    knowledgeBindingModes: knowledgeModesFromBindings(knowledgeEntries, knowledgeBindings),
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
    deprecated: v.deprecated,
    configured: v.configured,
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
  } as DeploymentTemplate;
}

function initialValuesFromTemplateResponse(
  resp: TemplateResponse,
  account: string,
  options: ComputeInitialValuesOptions = {},
): DeployFormInitialValues {
  const template = toDeploymentTemplate(resp);
  const extracted = computeInitialValues(template, account, resp.interfaces, resp.schedules, options);
  const respAgent = resp.provisioning?.agent;
  const knowledgeEntries = knowledgeEntriesFromTemplate(template);
  const knowledgeBindings = bindingsFromTemplateResponse(resp, template);

  return {
    ...extracted,
    knowledgeBindings,
    knowledgeBindingModes: knowledgeModesFromBindings(knowledgeEntries, knowledgeBindings),
    agentCpu: respAgent?.compute?.cpu ?? "",
    agentMemory: respAgent?.compute?.memory ?? "",
    agentVolumeMount: respAgent?.volume?.mount ?? "",
    agentStorageSize: respAgent?.volume?.storage?.size ?? "",
    agentResponseTimeout: respAgent?.response_timeout ?? "",
  };
}

const hasTextValue = (value: string | undefined): boolean => !!value?.trim();

/**
 * Builds the provisioning block from the form's advanced inputs. Returns
 * undefined when every field is empty so the server falls back to defaults.
 * Every agent gets a persistent disk by default (mounted at
 * DEFAULT_AGENT_VOLUME_MOUNT server-side), so there is no enable/disable toggle.
 * A volume override is sent when the user customizes the mount path or the
 * storage size; the mount falls back to the default path when left blank.
 */
function buildAgentProvisioning(input: {
  cpu: string;
  memory: string;
  mount: string;
  size: string;
  responseTimeout: string;
}): TemplateProvisioning | undefined {
  const cpu = input.cpu.trim();
  const memory = input.memory.trim();
  const mount = input.mount.trim();
  const size = input.size.trim();
  const responseTimeout = input.responseTimeout.trim();
  const compute = (cpu || memory) ? { ...(cpu && { cpu }), ...(memory && { memory }) } : undefined;
  const volume = (mount || size)
    ? { mount: mount || DEFAULT_AGENT_VOLUME_MOUNT, ...(size && { storage: { size } }) }
    : undefined;
  if (!compute && !volume && !responseTimeout) return undefined;
  return {
    agent: {
      ...(compute && { compute }),
      ...(volume && { volume }),
      ...(responseTimeout && { response_timeout: responseTimeout }),
    },
  };
}

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

/** Stable key for the server template bootstrap POST (identity + deploy pin). */
function templateBootstrapKey(
  account: string,
  name: string,
  deploymentId: string | undefined,
  build: string | undefined,
  revision: number | undefined,
): string {
  return [
    account,
    name,
    deploymentId ?? '',
    build ?? '',
    revision === undefined ? '' : String(revision),
  ].join('\0');
}

/** Default grants for a fresh deploy, per adapter:
 *   - web   → the deploying user, so they can use it on day one
 *   - slack → anyone, matching how Slack apps typically install (workspace-wide)
 *  Returns [] when no sensible default applies (e.g. web with no signed-in user). */
export function defaultGrantsForAdapter(
  adapter: "web" | "slack" | "custom",
  userId: string | undefined,
): AuthGrant[] {
  // Slack defaults to the whole workspace; the custom interface defaults to any
  // signed-in Astro account. Web defaults to just the deploying user.
  if (adapter === "slack" || adapter === "custom") return [{ anyone: true }];
  if (adapter === "web" && userId) return [{ user_id: userId }];
  return [];
}

/** Stable string key for an auth grant, used for comparison and React lists. */
export function grantKey(g: AuthGrant): string {
  if (g.anyone) return 'anyone';
  if (g.user_id) return `user:${g.user_id}`;
  if (g.org) return `org:${g.org}`;
  return 'unset';
}

/** Count add+remove diffs between two grant lists, order-insensitive. */
function diffGrants(current: AuthGrant[], initial: AuthGrant[]): number {
  const a = new Set(current.map(grantKey));
  const b = new Set(initial.map(grantKey));
  let count = 0;
  for (const k of a) if (!b.has(k)) count++;
  for (const k of b) if (!a.has(k)) count++;
  return count;
}

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
  knowledgeBindings?: string[];
}

// --- Hook ---

export function useDeployForm(account: string, name: string, opts?: UseDeployFormOptions) {
  const { accounts, personalAccount, user } = useAuth();
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
    isPlaceholderData: accountVarsPlaceholder,
    isError: vaultVarsQueryFailed,
    error: vaultVarsQueryError,
  } = useAccountVariables(targetAccount);
  const accountVarsReady = !!targetAccount && accountVarsLoaded && !accountVarsPlaceholder;
  const vaultEntriesLoadError = vaultVarsQueryFailed
    ? formatVaultVariablesLoadError(vaultVarsQueryError)
    : null;
  const accountVarNames = useMemo(
    () => new Set(accountVarsReady ? accountVarsData?.variables.map(v => v.name) ?? [] : []),
    [accountVarsData?.variables, accountVarsReady],
  );

  const [initialValues, setInitialValues] = useState<DeployFormInitialValues | null>(null);
  const seededRef = useRef(false);

  // Fetch template via interactive POST endpoint.
  const templateMutation = usePostDeploymentTemplate(account, name);
  const [templateResponse, setTemplateResponse] = useState<TemplateResponse | null>(
    opts?.initialTemplateResponse ?? null,
  );
  const lastBootstrapKeyRef = useRef<string | null>(
    opts?.initialTemplateResponse
      ? templateBootstrapKey(account, name, opts?.deploymentId, opts?.build, opts?.revision)
      : null,
  );
  const [fetchError, setFetchError] = useState<Error | null>(null);

  useEffect(() => {
    if (!account || !name) return;

    const bootstrapKey = templateBootstrapKey(
      account,
      name,
      opts?.deploymentId,
      opts?.build,
      opts?.revision,
    );
    if (lastBootstrapKeyRef.current === bootstrapKey) return;

    lastBootstrapKeyRef.current = bootstrapKey;
    seededRef.current = false;
    setTemplateResponse(null);
    setFetchError(null);
    const body: TemplateRequest = {};
    if (opts?.deploymentId) body.deployment_id = opts.deploymentId;
    if (opts?.build) body.build = opts.build;
    if (opts?.revision !== undefined) body.revision = opts.revision;
    void templateMutation.mutateAsync(body).then(setTemplateResponse, setFetchError);
  }, [account, name, opts?.deploymentId, opts?.build, opts?.revision, templateMutation]);

  const templateLoading = !templateResponse && !fetchError;
  const templateError = fetchError;

  // Derive legacy DeploymentTemplate shape for existing form logic.
  const template: DeploymentTemplate | null = useMemo(() => {
    if (templateResponse) return toDeploymentTemplate(templateResponse);
    return null;
  }, [templateResponse]);

  // The chat-interface picker and its "at least one adapter" validation are
  // gated on messaging support (keyed off the sidecar image). The custom
  // interface section is gated independently on whether the agent exposes its
  // own endpoint — the two can both show, neither, or one.
  const messagingSupported = agentHasMessaging(template);
  const customSupported = agentHasCustomInterface(template);
  const requiresMessagingAdapter = messagingSupported && !customSupported;

  const deployMutation = useDeployAgent(targetAccount, name);

  // Compute initial form state synchronously so the form is correct on the
  // first render. When initialValues are provided (settings page), use those.
  // Otherwise, derive defaults from the template (fresh deploy page).
  // The POST-based seeding effect will override these once the template loads.
  // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally computed once at mount
  const computedDefaults = useMemo(() => {
    if (iv) return iv;
    if (opts?.initialTemplateResponse) {
      return initialValuesFromTemplateResponse(
        opts.initialTemplateResponse,
        account,
        { preserveEmptyAdapters: !!opts?.deploymentId },
      );
    }
    return computeFormDefaults(null, name);
  }, []);

  const [deployName, setDeployName] = useState(() => computedDefaults.deployName ?? slugToTitle(name));
  const [variableValues, setVariableValues] = useState<Record<string, string>>(computedDefaults.variableValues ?? {});
  const [selectedAdapters, setSelectedAdaptersRaw] = useState<string[]>(computedDefaults.selectedAdapters ?? ["web"]);
  const [adapterCredentials, setAdapterCredentials] = useState<Record<string, string>>(computedDefaults.adapterCredentials ?? {});
  const [webGrants, setWebGrants] = useState<AuthGrant[]>(computedDefaults.webGrants ?? []);
  const [slackGrants, setSlackGrants] = useState<AuthGrant[]>(computedDefaults.slackGrants ?? []);
  const [customPublic, setCustomPublic] = useState<boolean>(computedDefaults.customPublic ?? false);
  const [customGrants, setCustomGrants] = useState<AuthGrant[]>(computedDefaults.customGrants ?? []);
  const [ingestionSchedules, setIngestionSchedules] = useState<Record<string, string>>(computedDefaults.ingestionSchedules ?? {});
  const [knowledgeBindings, setKnowledgeBindingsRaw] = useState<Record<string, string>>(computedDefaults.knowledgeBindings ?? {});
  const [knowledgeBindingModes, setKnowledgeBindingModesRaw] = useState<KnowledgeBindingModes>(
    () => computedDefaults.knowledgeBindingModes ?? knowledgeModesFromBindings({}, computedDefaults.knowledgeBindings ?? {}),
  );
  // Advanced provisioning overrides — all optional; empty strings let the
  // server fall back to astropods.yml declarations and tier defaults.
  const [agentCpu, setAgentCpu] = useState<string>(computedDefaults.agentCpu ?? "");
  const [agentMemory, setAgentMemory] = useState<string>(computedDefaults.agentMemory ?? "");
  const [agentVolumeMount, setAgentVolumeMount] = useState<string>(computedDefaults.agentVolumeMount ?? "");
  const [agentStorageSize, setAgentStorageSize] = useState<string>(computedDefaults.agentStorageSize ?? "");
  const [agentResponseTimeout, setAgentResponseTimeout] = useState<string>(computedDefaults.agentResponseTimeout ?? "");
  // True once we observe that the deployment loaded from the server already
  // has a persistent volume attached. K8s won't resize a live PVC in place
  // (the StatefulSet's volumeClaimTemplates is immutable on update), so once
  // this flips true we lock the storage slider in the UI.
  const [volumeAlreadyProvisioned, setVolumeAlreadyProvisioned] = useState(
    () => !!opts?.deploymentId && !!opts?.initialTemplateResponse?.provisioning?.agent?.volume?.mount,
  );
  const [deployError, setDeployError] = useState<{ message: string; details?: string } | null>(null);
  const [submitted, setSubmitted] = useState(false);

  // Applies a set of form values to all state variables at once.
  // Used by both the initial seeding effect and `reset()`.
  //
  // targetAccount is only set when explicitly provided: callers that need to
  // lock the picker to a specific account (configure page redeploys, private
  // blueprint deploys) pass `allowedTargetAccounts` instead. Seeding from the
  // blueprint/source account here would override the user's personal-account
  // default for public blueprint deploys and cause the vault variables lookup
  // to query the blueprint owner's account.
  const applyValues = (v: DeployFormInitialValues) => {
    // deployName uses || because "" should fall through to the slugToTitle fallback
    setDeployName(v.deployName || slugToTitle(name));
    setVariableValues(v.variableValues ?? {});
    setSelectedAdaptersRaw(v.selectedAdapters ?? ["web"]);
    setAdapterCredentials(v.adapterCredentials ?? {});
    setIngestionSchedules(v.ingestionSchedules ?? {});
    setWebGrants(v.webGrants ?? []);
    setSlackGrants(v.slackGrants ?? []);
    setCustomPublic(v.customPublic ?? false);
    setCustomGrants(v.customGrants ?? []);
    const nextKnowledgeBindings = v.knowledgeBindings ?? {};
    setKnowledgeBindingsRaw(nextKnowledgeBindings);
    setKnowledgeBindingModesRaw(
      knowledgeModesFromBindings(
        knowledgeEntriesFromTemplate(template),
        nextKnowledgeBindings,
        v.knowledgeBindingModes,
      ),
    );
    setAgentCpu(v.agentCpu ?? "");
    setAgentMemory(v.agentMemory ?? "");
    setAgentVolumeMount(v.agentVolumeMount ?? "");
    setAgentStorageSize(v.agentStorageSize ?? "");
    setAgentResponseTimeout(v.agentResponseTimeout ?? "");
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

    const extracted = templateResponse
      ? initialValuesFromTemplateResponse(
          templateResponse,
          account,
          { preserveEmptyAdapters: !!opts?.deploymentId },
        )
      : computeInitialValues(template, account, undefined, undefined, { preserveEmptyAdapters: !!opts?.deploymentId });

    // Seed advanced provisioning from the resolved response so the user sees
    // the effective values (whether they came from the request, the astropods
    // declaration, or tier defaults).
    const respAgent = templateResponse?.provisioning?.agent;
    const seededAgentCpu = extracted.agentCpu ?? "";
    const seededAgentMemory = extracted.agentMemory ?? "";
    const seededAgentVolumeMount = extracted.agentVolumeMount ?? "";
    const seededAgentStorageSize = extracted.agentStorageSize ?? "";
    const seededAgentResponseTimeout = extracted.agentResponseTimeout ?? "";
    // For an existing deployment, treat a volume returned by the template as
    // already provisioned in the cluster — its size is locked from here on.
    if (opts?.deploymentId && respAgent?.volume?.mount) {
      setVolumeAlreadyProvisioned(true);
    }
    // Fresh-deploy defaults: when there's no existing deployment to configure
    // and the template returned no grants, seed an adapter-appropriate default
    // so the form doesn't start in the "no one has access" state. Web defaults
    // to the deploying user; Slack defaults to anyone (workspace-wide), which
    // matches how Slack apps are typically installed.
    const isFreshDeploy = !opts?.deploymentId && !iv;
    const seededWebGrants = extracted.webGrants && extracted.webGrants.length > 0
      ? extracted.webGrants
      : isFreshDeploy ? defaultGrantsForAdapter("web", user?.id) : [];
    const seededSlackGrants = extracted.slackGrants && extracted.slackGrants.length > 0
      ? extracted.slackGrants
      : isFreshDeploy ? defaultGrantsForAdapter("slack", user?.id) : [];
    const seededCustomGrants = extracted.customGrants && extracted.customGrants.length > 0
      ? extracted.customGrants
      : isFreshDeploy ? defaultGrantsForAdapter("custom", user?.id) : [];

    const merged: DeployFormInitialValues = {
      deployName: iv?.deployName || extracted.deployName || slugToTitle(name),
      // Don't seed from extracted.targetAccount (the blueprint/source account):
      // for public blueprint deploys the user's personal-account default must
      // win so vault variables are looked up under the deploying user's account.
      // Account locking for redeploy/private-blueprint flows is handled by
      // `allowedTargetAccounts` instead.
      targetAccount: iv?.targetAccount,
      variableValues: { ...extracted.variableValues, ...(iv?.variableValues ?? {}) },
      selectedAdapters: iv?.selectedAdapters ?? extracted.selectedAdapters ?? ["web"],
      adapterCredentials: { ...extracted.adapterCredentials, ...(iv?.adapterCredentials ?? {}) },
      ingestionSchedules: { ...extracted.ingestionSchedules, ...(iv?.ingestionSchedules ?? {}) },
      webGrants: iv?.webGrants ?? seededWebGrants,
      slackGrants: iv?.slackGrants ?? seededSlackGrants,
      customPublic: iv?.customPublic ?? extracted.customPublic ?? false,
      customGrants: iv?.customGrants ?? seededCustomGrants,
      knowledgeBindings: iv?.knowledgeBindings ?? extracted.knowledgeBindings ?? {},
      knowledgeBindingModes: knowledgeModesFromBindings(
        knowledgeEntriesFromTemplate(template),
        iv?.knowledgeBindings ?? extracted.knowledgeBindings ?? {},
        iv?.knowledgeBindingModes ?? extracted.knowledgeBindingModes,
      ),
      agentCpu: iv?.agentCpu ?? seededAgentCpu,
      agentMemory: iv?.agentMemory ?? seededAgentMemory,
      agentVolumeMount: iv?.agentVolumeMount ?? seededAgentVolumeMount,
      agentStorageSize: iv?.agentStorageSize ?? seededAgentStorageSize,
      agentResponseTimeout: iv?.agentResponseTimeout ?? seededAgentResponseTimeout,
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
    if (opts?.revision !== undefined) body.revision = opts.revision;
    templateMutation.mutateAsync(body).then(setTemplateResponse, setFetchError);
  // eslint-disable-next-line react-hooks/exhaustive-deps -- stable identity for opts
  }, [opts?.deploymentId, opts?.build, opts?.revision]);

  // Build the interfaces payload from current form state. Web auth is always
  // OIDC when web is selected; grants are emitted only for selected adapters
  // so deselecting an adapter doesn't push stale grants to the server.
  const buildInterfaces = useCallback((): TemplateInterfaces => {
    const auth: TemplateInterfaces['auth'] = {};
    if (selectedAdapters.includes('web')) {
      auth.web = { type: 'oidc', grants: webGrants };
    }
    if (selectedAdapters.includes('slack')) {
      auth.slack = { grants: slackGrants };
    }
    // Custom is not a messaging adapter — emit it whenever the agent ships a
    // custom interface, independent of the selected adapters.
    if (customSupported) {
      auth.custom = { public: customPublic, grants: customGrants };
    }
    return {
      adapters: selectedAdapters,
      auth: auth.web || auth.slack || auth.custom ? auth : undefined,
    };
  }, [selectedAdapters, webGrants, slackGrants, customSupported, customPublic, customGrants]);

  // Exposed adapter setter: updates local state AND re-triggers template shaping
  // so the server can flip variable optionality (e.g. Slack tokens become required).
  // bindings.knowledge is always emitted (even when empty) — see setKnowledgeBindings for why.
  //
  // On a fresh deploy, enabling an adapter whose grants are currently empty
  // seeds the same per-adapter default the initial seeding effect uses. This
  // keeps the "turn it on, ship it" path consistent whether the user lands on
  // the form with that adapter pre-selected or toggles it on later.
  const setSelectedAdapters = useCallback((adapters: string[]) => {
    const isFreshDeploy = !opts?.deploymentId && !iv;
    const newlyEnabled = adapters.filter((a) => !selectedAdapters.includes(a));

    let nextWebGrants = webGrants;
    let nextSlackGrants = slackGrants;
    if (isFreshDeploy) {
      if (newlyEnabled.includes("web") && webGrants.length === 0) {
        nextWebGrants = defaultGrantsForAdapter("web", user?.id);
        if (nextWebGrants.length > 0) setWebGrants(nextWebGrants);
      }
      if (newlyEnabled.includes("slack") && slackGrants.length === 0) {
        nextSlackGrants = defaultGrantsForAdapter("slack", user?.id);
        if (nextSlackGrants.length > 0) setSlackGrants(nextSlackGrants);
      }
    }

    setSelectedAdaptersRaw(adapters);
    const auth: TemplateInterfaces['auth'] = {};
    if (adapters.includes('web')) auth.web = { type: 'oidc', grants: nextWebGrants };
    if (adapters.includes('slack')) auth.slack = { grants: nextSlackGrants };
    // Preserve the custom-interface auth across adapter toggles.
    if (customSupported) auth.custom = { public: customPublic, grants: customGrants };
    reshapeTemplate({
      interfaces: { adapters, auth: auth.web || auth.slack || auth.custom ? auth : undefined },
      bindings: { knowledge: nonEmptyBindings(knowledgeBindings) },
    });
  }, [reshapeTemplate, webGrants, slackGrants, customSupported, customPublic, customGrants, knowledgeBindings, selectedAdapters, opts?.deploymentId, iv, user?.id]);

  // Filter empty-string ARNs — an entry with value "" means "not bound".
  const nonEmptyBindings = (b: Record<string, string>): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(b)) {
      if (v) out[k] = v;
    }
    return out;
  };

  // Exposed binding setter: updates state and re-POSTs to reshape the template.
  // Binding selection is a structural change (removes/adds knowledge entries and variables).
  // Always send a (possibly empty) knowledge map so the server preserves
  // explicit clears — sending `bindings: undefined` would let the server
  // restore stored bindings on top of a user who cleared all of them.
  const setKnowledgeBindings = useCallback((bindings: Record<string, string>) => {
    const cleaned = nonEmptyBindings(bindings);
    setKnowledgeBindingsRaw(cleaned);
    setKnowledgeBindingModesRaw((prev) =>
      knowledgeModesFromBindings(knowledgeEntriesFromTemplate(template), cleaned, prev),
    );
    reshapeTemplate({
      interfaces: buildInterfaces(),
      bindings: { knowledge: cleaned },
    });
  }, [reshapeTemplate, buildInterfaces, template]);

  const setKnowledgeBindingMode = useCallback((entryName: string, mode: KnowledgeBindingMode) => {
    setKnowledgeBindingModesRaw((prev) => ({ ...prev, [entryName]: mode }));
    if (mode === "shared") return;

    const nextBindings = { ...knowledgeBindings };
    delete nextBindings[entryName];
    setKnowledgeBindingsRaw(nextBindings);
    reshapeTemplate({
      interfaces: buildInterfaces(),
      bindings: { knowledge: nextBindings },
    });
  }, [buildInterfaces, knowledgeBindings, reshapeTemplate]);

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
            deprecated: fieldDef.deprecated,
          }]);
        }
      }
      displayDefs[adapterId] = subFields.length > 0
        ? [...realFields, ...subFields]
        : realFields;
    }

    return { adapterVariableDefs: varDefs, adapterDisplayFields: displayDefs };
  }, [template, selectedAdapters]);

  const vaultRefKeys = useMemo(
    () => variableEntries
      .filter(([key]) => parseVaultToken(allFormValues[key] ?? '') !== null)
      .map(([key]) => key),
    [variableEntries, allFormValues],
  );

  // Always-on vault ref validation — not gated by submitted so chips turn red
  // immediately when the target account changes, without requiring a submit attempt.
  const invalidVaultRefKeys = useMemo(
    () => accountVarsReady
      ? variableEntries
          .filter(([key]) => isInvalidVaultRef(allFormValues[key] ?? '', accountVarNames))
          .map(([key]) => key)
      : [],
    [accountVarsReady, variableEntries, allFormValues, accountVarNames],
  );
  const missingSharedKnowledgeBindings = useMemo(
    () => sharedKnowledgeEntriesMissingBinding(
      knowledgeEntriesFromTemplate(template),
      knowledgeBindings,
      knowledgeBindingModes,
    ),
    [template, knowledgeBindings, knowledgeBindingModes],
  );

  const trimmedDeployName = deployName.trim();
  const deployNameLength = Array.from(trimmedDeployName).length;
  const deployNameLengthError = deployNameLength > DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH
    ? `Name must be ${DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH} characters or fewer`
    : undefined;

  // Compute validation errors. Length is shown while typing; required fields
  // stay submit-gated so the untouched form does not start in an error state.
  const errors = useMemo<FormErrors>(() => {
    const result: FormErrors = {};
    if (deployNameLengthError) {
      result.deployName = deployNameLengthError;
    }

    if (!submitted) return result;

    if (!targetAccount) {
      result.account = "Select an account to install under";
    }

    if (!trimmedDeployName) {
      result.deployName = "Enter a name for the agent";
    }

    if (requiresMessagingAdapter && selectedAdapters.length === 0) {
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

    if (missingSharedKnowledgeBindings.length > 0) {
      result.knowledgeBindings = missingSharedKnowledgeBindings;
    }

    return result;
  }, [submitted, targetAccount, trimmedDeployName, deployNameLengthError, selectedAdapters, requiresMessagingAdapter, requiredVariables, allFormValues, adapterDisplayFields, scheduleIngestions, ingestionSchedules, invalidVaultRefKeys, missingSharedKnowledgeBindings]);

  const isValid = submitted
    ? !errors.account && !errors.deployName && !errors.adapters && !errors.credentials && !errors.adapterCredentials && !errors.ingestionSchedules && !errors.knowledgeBindings
    : !errors.deployName;

  // Try to submit: marks form as submitted and returns validity
  const trySubmit = (): boolean => {
    setSubmitted(true);

    // Compute validity inline (state update is async, can't rely on `errors` yet)
    const hasAccount = !!targetAccount;
    const hasName = !!trimmedDeployName;
    const nameLengthValid = deployNameLength <= DEPLOYMENT_DISPLAY_NAME_MAX_LENGTH;
    const hasAdapter = !requiresMessagingAdapter || selectedAdapters.length > 0;
    const varsValid = requiredVariables.every(([key, v]) => isVariableFilled(v, allFormValues[key]));
    const adapterCredsValid = selectedAdapters.every((adapterId) => {
      const creds = adapterDisplayFields[adapterId] ?? [];
      return creds.every(([key, def]) => def.optional || isVariableFilled(def, allFormValues[key]));
    });
    const schedulesValid = scheduleIngestions.every((n) => ingestionSchedules[n]?.trim());
    const vaultRefsValid = vaultRefKeys.length === 0
      || (accountVarsReady && invalidVaultRefKeys.length === 0);
    const knowledgeBindingsValid = missingSharedKnowledgeBindings.length === 0;

    return hasAccount && hasName && nameLengthValid && hasAdapter && varsValid && adapterCredsValid && schedulesValid && vaultRefsValid && knowledgeBindingsValid;
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
    if (opts?.revision !== undefined) req.revision = opts.revision;
    const provisioning = buildAgentProvisioning({
      cpu: agentCpu,
      memory: agentMemory,
      mount: agentVolumeMount,
      size: agentStorageSize,
      responseTimeout: agentResponseTimeout,
    });
    if (provisioning) req.provisioning = provisioning;
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

    // Knowledge bindings — count one change per entry whose mode or selected
    // shared store differs from the deployed form state.
    const ivBindings = initialValues.knowledgeBindings ?? {};
    const ivModes = initialValues.knowledgeBindingModes ?? knowledgeModesFromBindings(
      knowledgeEntriesFromTemplate(template),
      ivBindings,
    );
    deployCount += knowledgeBindingChangeCount(
      { bindings: ivBindings, modes: ivModes },
      { bindings: knowledgeBindings, modes: knowledgeBindingModes },
    );

    // Auth grants — count adds/removes per adapter
    deployCount += diffGrants(webGrants, initialValues.webGrants ?? []);
    deployCount += diffGrants(slackGrants, initialValues.slackGrants ?? []);
    deployCount += diffGrants(customGrants, initialValues.customGrants ?? []);
    if (customPublic !== (initialValues.customPublic ?? false)) deployCount++;

    deployCount += provisioningChangeCount(
      {
        agentCpu: initialValues.agentCpu ?? "",
        agentMemory: initialValues.agentMemory ?? "",
        agentVolumeMount: initialValues.agentVolumeMount ?? "",
        agentStorageSize: initialValues.agentStorageSize ?? "",
        agentResponseTimeout: initialValues.agentResponseTimeout ?? "",
      },
      {
        agentCpu,
        agentMemory,
        agentVolumeMount,
        agentStorageSize,
        agentResponseTimeout,
      },
    );

    const deployChanged = deployCount > 0;
    const changeCount = (nameChanged ? 1 : 0) + deployCount;

    return { nameChanged, deployChanged, isDirty: nameChanged || deployChanged, changeCount };
  }, [initialValues, deployName, name, variableValues, selectedAdapters, adapterCredentials, webGrants, slackGrants, customGrants, customPublic, agentCpu, agentMemory, agentVolumeMount, agentStorageSize, agentResponseTimeout, ingestionSchedules, knowledgeBindings, knowledgeBindingModes, template]);

  const deferredDirty = useDeferredValue({ nameChanged, deployChanged, isDirty, changeCount });

  return {
    template,
    templateLoading,
    templateErrorMessage,
    serverValidation: templateResponse?.validation ?? null,
    initialValues,
    isExistingDeployment: !!opts?.deploymentId,
    /**
     * True when the deployment loaded from the server already has a
     * persistent volume — storage size cannot be resized in place, so the
     * UI must lock the slider. False on a fresh deploy *and* on an existing
     * deployment that doesn't have a volume yet (first-time enable is allowed).
     */
    volumeAlreadyProvisioned,

    nameChanged: deferredDirty.nameChanged,
    deployChanged: deferredDirty.deployChanged,
    isDirty: deferredDirty.isDirty,
    changeCount: deferredDirty.changeCount,

    accounts: selectableAccounts,
    targetAccount,
    setTargetAccount: setAllowedTargetAccount,

    deployName,
    setDeployName,

    messagingSupported,
    selectedAdapters,
    setSelectedAdapters,
    adapterDisplayFields,
    adapterCredentials,
    setAdapterCredentials,
    webGrants,
    setWebGrants,
    slackGrants,
    setSlackGrants,
    customSupported,
    customPublic,
    setCustomPublic,
    customGrants,
    setCustomGrants,

    variableValues,
    setVariableValues,
    requiredVariables,
    optionalVariables,

    scheduleIngestions,
    ingestionSchedules,
    setIngestionSchedules,

    knowledgeBindings,
    knowledgeBindingModes,
    setKnowledgeBindings,
    setKnowledgeBindingMode,
    resolvedBindings: templateResponse?.bindings?.knowledge ?? {},
    knowledgeEntries: template?.knowledge as Record<string, { provider?: string; binding?: string }> | undefined,

    // Advanced provisioning overrides
    agentCpu,
    setAgentCpu,
    agentMemory,
    setAgentMemory,
    agentVolumeMount,
    setAgentVolumeMount,
    agentStorageSize,
    setAgentStorageSize,
    agentResponseTimeout,
    setAgentResponseTimeout,

    vaultEntries: accountVarsReady ? accountVarsData?.variables ?? [] : [],
    vaultEntriesLoaded: accountVarsReady,
    vaultEntriesLoadError,
    vaultSettingsUrl: accountSettingsPath(accounts, targetAccount, 'secrets'),

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
