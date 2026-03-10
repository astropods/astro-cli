/**
 * @deprecated Legacy operator deploy page (route: operator/deploy/:account/:name).
 * Superseded by InstallAgent (route: deploy/:account/:agentSlug).
 * Do not add new features here.
 */
import { useState, useEffect, useMemo } from "react";
import { useParams, useNavigate } from "react-router";
import {
  ArrowLeft,
  Rocket,
  RefreshCw,
  Loader2,
  Server,
  Brain,
  Database,
  Wrench,
  Download,
  Layout,
  Activity,
  Lock,
  MessageSquare,
  Check,
  CheckCircle,
  AlertCircle,
  Info,
  ShieldCheck,
} from "lucide-react";
import type {
  DeploymentTemplate,
  DeploymentVariable,
  DeploymentSpec,
  ApiError,
} from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { useDeploymentTemplate, useDeployAgent, useValidateDeployment } from "../../api/queries/agents";
import { useDeployments } from "../../api/queries/deployments";

// --- Available adapters (server validates against this same set) ---

const AVAILABLE_ADAPTERS: { id: string; label: string; description: string }[] = [
  { id: "web", label: "Web", description: "Browser-based chat interface" },
  { id: "slack", label: "Slack", description: "Slack bot via socket mode" },
];

const ADAPTER_CREDENTIALS: Record<string, { key: string; description: string; secret?: boolean }[]> = {
  slack: [
    { key: "SLACK_BOT_TOKEN", description: "Slack bot token for messaging", secret: true },
    { key: "SLACK_APP_TOKEN", description: "Slack app token for socket mode", secret: true },
  ],
};

// --- Legacy helpers ---

/** @deprecated */
function formatResources(resources: Record<string, unknown> | undefined): string {
  if (!resources) return "";
  const parts: string[] = [];
  if (resources.cpu) parts.push(`CPU: ${resources.cpu}`);
  if (resources.memory) parts.push(`Mem: ${resources.memory}`);
  if (resources.gpu) parts.push(`GPU: ${resources.gpu}`);
  return parts.join(" / ");
}

/** @deprecated */
function KV({ label, value }: { label: string; value: string | number | undefined | null }) {
  if (value === undefined || value === null || value === "") return null;
  return (
    <div className="flex items-start gap-2 text-xs">
      <span className="text-stone-500 shrink-0 w-[72px]">{label}</span>
      <span className="font-mono text-stone-800 break-all">{String(value)}</span>
    </div>
  );
}

// --- Legacy components ---

/** @deprecated */
function ComponentCard({ name, config }: { name: string; config: Record<string, unknown> }) {
  const image = config.image as string | undefined;
  const port = config.port as number | undefined;
  const replicas = config.replicas as number | undefined;
  const resources = config.resources as Record<string, unknown> | undefined;
  const gpu = config.gpu as Record<string, unknown> | undefined;
  const healthcheck = config.healthcheck as Record<string, unknown> | undefined;
  const persistent = config.persistent as boolean | undefined;
  const storage = config.storage as string | undefined;
  const trigger = config.trigger as { type?: string; schedule?: string } | string | undefined;
  const expose = config.expose as Record<string, unknown> | undefined;
  const update_strategy = config.update_strategy as string | undefined;
  const env = config.env as Record<string, unknown> | undefined;

  return (
    <div className="bg-stone-50 border border-stone-200 p-3 space-y-0.5">
      <h4 className="font-medium text-sm text-stone-900 mb-1">{name}</h4>
      <KV label="Image" value={image} />
      <KV label="Port" value={port || undefined} />
      <KV label="Replicas" value={replicas} />
      {resources && <KV label="Resources" value={formatResources(resources)} />}
      {gpu && <KV label="GPU" value={formatResources(gpu)} />}
      {healthcheck && <KV label="Health" value={healthcheck.path as string} />}
      {persistent !== undefined && <KV label="Persistent" value={persistent ? "Yes" : "No"} />}
      <KV label="Storage" value={storage} />
      {trigger && typeof trigger === "object" ? (
        <>
          <KV label="Trigger" value={trigger.type} />
          <KV label="Schedule" value={trigger.schedule} />
          {trigger.type === "webhook" && !port && (
            <div className="flex items-start gap-2 text-xs">
              <span className="text-stone-500 shrink-0 w-[72px]">Port</span>
              <span className="text-red-600">required for webhook triggers</span>
            </div>
          )}
        </>
      ) : (
        <KV label="Trigger" value={trigger as string} />
      )}
      <KV label="Strategy" value={update_strategy} />
      {expose && <KV label="Expose" value={expose.type as string} />}
      {env && Object.keys(env).length > 0 && (
        <KV label="Env" value={`${Object.keys(env).length} var${Object.keys(env).length !== 1 ? "s" : ""}`} />
      )}
    </div>
  );
}

/** @deprecated */
function Section({ icon, title, items }: { icon: React.ReactNode; title: string; items: Record<string, unknown> | undefined }) {
  if (!items || Object.keys(items).length === 0) return null;

  return (
    <div>
      <div className="flex items-center gap-2 mb-2">
        {icon}
        <span className="text-sm font-medium text-stone-700">{title}</span>
        <span className="text-xs text-stone-400">({Object.keys(items).length})</span>
      </div>
      <div className="grid gap-2">
        {Object.entries(items).map(([name, config]) => (
          <ComponentCard key={name} name={name} config={config as Record<string, unknown>} />
        ))}
      </div>
    </div>
  );
}

/** @deprecated */
function InterfacesPicker({
  selected,
  onChange,
  adapterCredDefs,
  adapterCredentials,
  onCredentialChange,
}: {
  selected: string[];
  onChange: (adapters: string[]) => void;
  adapterCredDefs: Record<string, { key: string; description: string; secret?: boolean }[]>;
  adapterCredentials: Record<string, string>;
  onCredentialChange: (values: Record<string, string>) => void;
}) {
  const toggle = (id: string) => {
    onChange(selected.includes(id) ? selected.filter((a) => a !== id) : [...selected, id]);
  };

  return (
    <div>
      <div className="flex items-center gap-2 mb-2">
        <Layout size={16} className="text-indigo-600" />
        <span className="text-sm font-medium text-stone-700">Interfaces</span>
        {selected.length > 0 && (
          <span className="text-xs text-stone-400">({selected.length} selected)</span>
        )}
      </div>
      <p className="text-xs text-stone-500 mb-3">
        Each selected adapter deploys a messaging container alongside your agent.
      </p>
      <div className="space-y-2">
        {AVAILABLE_ADAPTERS.map((adapter) => {
          const isSelected = selected.includes(adapter.id);
          return (
            <button
              key={adapter.id}
              type="button"
              onClick={() => toggle(adapter.id)}
              className={`w-full flex items-center gap-3 p-3 border text-left cursor-pointer transition-colors ${
                isSelected
                  ? "border-indigo-400 bg-indigo-50"
                  : "border-stone-200 bg-stone-50 hover:bg-stone-100"
              }`}
            >
              <div className={`w-4 h-4 border flex items-center justify-center shrink-0 ${
                isSelected ? "border-indigo-500 bg-indigo-500" : "border-stone-300 bg-white"
              }`}>
                {isSelected && <Check size={12} className="text-white" />}
              </div>
              <MessageSquare size={14} className="text-stone-400 shrink-0" />
              <div className="min-w-0">
                <span className="text-sm font-medium">{adapter.label}</span>
                <span className="text-xs text-stone-500 ml-2">{adapter.description}</span>
              </div>
            </button>
          );
        })}
      </div>

      {selected.map((adapterId) => {
        const creds = adapterCredDefs[adapterId];
        if (!creds || creds.length === 0) return null;
        return (
          <div key={adapterId} className="mt-3 p-3 bg-stone-50 border border-stone-200">
            <h4 className="text-xs font-medium text-stone-700 mb-2 flex items-center gap-1">
              <Lock size={10} />
              {adapterId.charAt(0).toUpperCase() + adapterId.slice(1)} Credentials
            </h4>
            <div className="space-y-2">
              {creds.map((cred) => (
                <div key={cred.key}>
                  <label className="block text-xs font-medium text-stone-600 mb-1">{cred.key}</label>
                  <input
                    type="password"
                    value={adapterCredentials[cred.key] || ""}
                    onChange={(e) => onCredentialChange({ ...adapterCredentials, [cred.key]: e.target.value })}
                    placeholder={cred.description}
                    className="w-full py-1.5 px-2 border border-stone-300 text-sm focus:outline-2 focus:outline-stone-800 focus:-outline-offset-2"
                  />
                  <p className="text-xs text-stone-400 mt-0.5">{cred.description}</p>
                </div>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  );
}

/** @deprecated */
function ObservabilityToggle({
  enabled,
  onToggle,
  config,
}: {
  enabled: boolean;
  onToggle: (enabled: boolean) => void;
  config: Record<string, unknown> | undefined;
}) {
  const provider = (config?.provider as string) || "galileo";
  const image = config?.image as string | undefined;
  const port = (config?.port as number) || 4318;
  const resources = config?.resources as Record<string, unknown> | undefined;

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <Activity size={16} className="text-green-600" />
          <span className="text-sm font-medium text-stone-700">Observability</span>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          onClick={() => onToggle(!enabled)}
          className={`relative inline-flex h-5 w-9 items-center rounded-full border-none cursor-pointer transition-colors ${
            enabled ? "bg-green-600" : "bg-stone-300"
          }`}
        >
          <span
            className={`inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${
              enabled ? "translate-x-[18px]" : "translate-x-[3px]"
            }`}
          />
        </button>
      </div>
      {enabled && (
        <div className="bg-stone-50 border border-stone-200 p-3 space-y-0.5">
          <KV label="Provider" value={provider} />
          <KV label="Port" value={port} />
          {image && <KV label="Image" value={image} />}
          {resources && <KV label="Resources" value={formatResources(resources)} />}
        </div>
      )}
    </div>
  );
}

// --- Credential Form ---

function fulfillTemplate(
  template: DeploymentTemplate,
  variableValues: Record<string, string>,
  selectedAdapters: string[],
): DeploymentSpec {
  const fulfilled = JSON.parse(JSON.stringify(template)) as Record<string, unknown>;
  fulfilled.spec = 'deployment/v1';
  delete fulfilled.editable;
  const variables = (fulfilled.variables ?? {}) as Record<string, Record<string, unknown>>;
  for (const [key, v] of Object.entries(variables)) {
    v.value = variableValues[key] ?? '';
    delete v.default;
    delete v.description;
    delete v.datatype;
    delete v['display-as'];
    delete v.options;
  }
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
  fulfilled.variables = variables;
  if (fulfilled.interfaces) {
    (fulfilled.interfaces as Record<string, unknown>).adapters = selectedAdapters;
  }
  return fulfilled as unknown as DeploymentSpec;
}

function CredentialForm({
  variables,
  values,
  onChange,
}: {
  variables: Record<string, DeploymentVariable>;
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
}) {
  const entries = Object.entries(variables);
  const required = entries.filter(([, v]) => !v.optional);
  const optional = entries.filter(([, v]) => v.optional);

  const renderFields = (fields: [string, DeploymentVariable][]) =>
    fields.map(([key, variable]) => (
      <div key={key}>
        <label className="block text-xs font-medium text-stone-700 mb-1">{key}</label>
        <input
          type={variable.secret ? "password" : "text"}
          value={values[key] || ""}
          onChange={(e) => onChange({ ...values, [key]: e.target.value })}
          placeholder={variable.description || key}
          className="w-full py-1.5 px-2 border border-stone-300 text-sm focus:outline-2 focus:outline-stone-800 focus:-outline-offset-2"
        />
        {variable.description && <p className="text-xs text-stone-400 mt-0.5">{variable.description}</p>}
      </div>
    ));

  if (entries.length === 0) return null;

  return (
    <div className="space-y-3">
      {required.length > 0 && (
        <div>
          <h4 className="text-xs font-medium mb-2 text-stone-600 flex items-center gap-1">
            <Lock size={10} /> Required
          </h4>
          <div className="space-y-2">{renderFields(required)}</div>
        </div>
      )}
      {optional.length > 0 && (
        <div>
          <h4 className="text-xs font-medium mb-2 text-stone-600">Optional</h4>
          <div className="space-y-2">{renderFields(optional)}</div>
        </div>
      )}
    </div>
  );
}

// --- Deploy Page ---

export default function DeployPage() {
  const { account, name } = useParams<{ account: string; name: string }>();
  const navigate = useNavigate();
  const { personalAccount, isAuthenticated, login } = useAuth();
  const userAccount = personalAccount?.name ?? "";

  const {
    data: template,
    isLoading,
    error: templateError,
  } = useDeploymentTemplate(account ?? "", name ?? "");

  const deployMutation = useDeployAgent(userAccount);
  const validateMutation = useValidateDeployment();

  // Check for existing deployment to distinguish deploy vs redeploy
  const { data: deploymentsData } = useDeployments(userAccount, isAuthenticated);
  const existingDeployment = (deploymentsData?.deployments ?? []).find((dep) => dep.name === name);
  const isRedeploy = !!existingDeployment;

  const [credentialValues, setCredentialValues] = useState<Record<string, string>>({});
  const [selectedAdapters, setSelectedAdapters] = useState<string[]>(["web"]);
  const [adapterCredentials, setAdapterCredentials] = useState<Record<string, string>>({});
  const [observabilityEnabled, setObservabilityEnabled] = useState(true);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [validationResult, setValidationResult] = useState<{ valid: boolean; errors?: string[] } | null>(null);

  const error = templateError
    ? (templateError as unknown as ApiError).error_description ??
      (templateError as Error).message ??
      "Failed to load deployment template"
    : null;

  // Agent/ingestion-targeting variables
  const variableEntries = useMemo(
    () =>
      template?.variables
        ? Object.entries(template.variables).filter(([, v]) =>
            v.targets.some((t) => t === "agent" || t.startsWith("ingestion")),
          )
        : [],
    [template],
  );

  useEffect(() => {
    if (variableEntries.length === 0) return;
    setCredentialValues((prev) => {
      const initial: Record<string, string> = {};
      for (const [key, v] of variableEntries) {
        initial[key] = prev[key] ?? v.default ?? "";
      }
      return initial;
    });
  }, [variableEntries]);

  useEffect(() => {
    if (!template?.observability) return;
    const obs = template.observability as Record<string, unknown>;
    if (obs.enabled === undefined) return;
    setObservabilityEnabled(obs.enabled as boolean);
  }, [template]);

  const requiredVariables = variableEntries.filter(([, v]) => !v.optional);

  // Per-adapter credential field definitions derived from template, falling back to hardcoded.
  const adapterCredDefs: Record<string, { key: string; description: string; secret?: boolean }[]> = {};
  for (const adapter of AVAILABLE_ADAPTERS) {
    const hardcoded = ADAPTER_CREDENTIALS[adapter.id] ?? [];
    if (template?.variables) {
      const derived = Object.entries(template.variables).filter(([, v]) => {
        const variable = v as { targets?: string[] };
        return variable.targets?.some((t: string) => t === `interface.${adapter.id}`);
      });
      if (derived.length > 0) {
        adapterCredDefs[adapter.id] = derived.map(([key, v]) => {
          const variable = v as { description?: string; secret?: boolean };
          return {
            key,
            description: hardcoded.find((c) => c.key === key)?.description ?? variable.description ?? key,
            secret: variable.secret ?? true,
          };
        });
        continue;
      }
    }
    adapterCredDefs[adapter.id] = hardcoded;
  }

  const adapterCredsValid = selectedAdapters.every((adapterId) => {
    const creds = adapterCredDefs[adapterId] ?? [];
    return creds.every((c) => adapterCredentials[c.key]?.trim());
  });

  const canDeploy =
    !isLoading &&
    !deployMutation.isPending &&
    requiredVariables.every(([key]) => credentialValues[key]?.trim()) &&
    adapterCredsValid;

  const handleDeploy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!account || !name || !template) return;

    if (!isAuthenticated) {
      login();
      return;
    }

    setDeployError(null);
    const allVariableValues = { ...credentialValues, ...adapterCredentials };
    const spec = fulfillTemplate(template, allVariableValues, selectedAdapters);

    try {
      const result = await deployMutation.mutateAsync(spec);
      navigate(`/u/${account}/${name}`, { state: { deployResult: result } });
    } catch (err) {
      const apiErr = err as {
        error?: string;
        details?: string;
        validation_errors?: Array<{ field: string; message: string }>;
        missing_variables?: string[];
      };

      const messages: string[] = [];
      if (apiErr.validation_errors?.length) {
        for (const ve of apiErr.validation_errors) messages.push(`${ve.field}: ${ve.message}`);
      }
      if (apiErr.missing_variables?.length) {
        messages.push(`Missing variables: ${apiErr.missing_variables.join(", ")}`);
      }
      if (messages.length === 0) {
        messages.push(apiErr.details || apiErr.error || (err instanceof Error ? err.message : "Deployment failed"));
      }
      setDeployError(messages.join("\n"));
    }
  };

  const handleValidate = async () => {
    if (!account || !name || !template) return;
    if (!isAuthenticated) {
      login();
      return;
    }

    setValidationResult(null);
    setDeployError(null);
    const allVariableValues = { ...credentialValues, ...adapterCredentials };
    const spec = fulfillTemplate(template, allVariableValues, selectedAdapters);

    try {
      const result = await validateMutation.mutateAsync(spec);

      if (result.valid) {
        setValidationResult({ valid: true });
      } else {
        setValidationResult({
          valid: false,
          errors: result.validation_errors?.map((e: { field: string; message: string }) => `${e.field}: ${e.message}`) ?? ["Validation failed"],
        });
      }
    } catch (err) {
      const apiErr = err as {
        error?: string;
        details?: string;
        validation_errors?: Array<{ field: string; message: string }>;
      };
      const errors: string[] = [];
      if (apiErr.validation_errors?.length) {
        for (const ve of apiErr.validation_errors) errors.push(`${ve.field}: ${ve.message}`);
      }
      if (errors.length === 0) {
        errors.push(apiErr.details || apiErr.error || (err instanceof Error ? err.message : "Validation failed"));
      }
      setValidationResult({ valid: false, errors });
    }
  };

  return (
    <div className="flex flex-col h-[calc(100vh-64px)]">
      <form onSubmit={handleDeploy} className="flex flex-col flex-1 min-h-0">
        {/* Header */}
        <div className="shrink-0 px-6 pt-6 pb-4 md:px-8 md:pt-8 border-b border-stone-200">
          <button
            type="button"
            onClick={() => navigate("/operator")}
            className="flex items-center gap-1 text-sm text-stone-500 hover:text-stone-800 bg-transparent border-none cursor-pointer mb-3 px-0"
          >
            <ArrowLeft size={16} />
            Back to Home
          </button>
          <div className="flex items-end justify-between">
            <div>
              <h1 className="text-heading-1 mb-1">
                Deploy <span className="font-normal text-stone-500">{account}/</span>{name}
              </h1>
              {template && (
                <p className="text-sm text-stone-600 font-mono">Build: {template.source.build}</p>
              )}
            </div>
            {template && (
              <div className="flex gap-4 text-sm text-stone-500">
                <span>Runtime: <span className="font-mono text-stone-700">{template.target.runtime}</span></span>
                <span>Namespace: <span className="font-mono text-stone-700">{template.target.namespace}</span></span>
              </div>
            )}
          </div>
        </div>

        {/* Redeploy info banner */}
        {isRedeploy && existingDeployment && (
          <div className="shrink-0 mx-6 md:mx-8 mt-4 p-3 bg-blue-50 border border-blue-200 text-blue-700 text-sm flex items-center gap-2">
            <Info size={16} className="shrink-0" />
            <span>
              This will update the existing deployment in namespace <code className="font-mono font-medium">{existingDeployment.namespace}</code>. Persistent data (volumes, secrets) will be preserved.
            </span>
          </div>
        )}

        {/* Loading / Error */}
        {isLoading && (
          <div className="flex items-center justify-center py-16 flex-1">
            <Loader2 size={24} className="animate-spin text-stone-500" />
            <span className="ml-2 text-stone-600">Loading deployment template...</span>
          </div>
        )}
        {error && (
          <div className="p-4 m-6 bg-red-50 border border-red-200 text-red-700 text-sm">{error}</div>
        )}

        {/* Two-pane body — 50/50 */}
        {template && !isLoading && (
          <div className="flex-1 min-h-0 grid grid-cols-2">
            {/* Left — Architecture */}
            <div className="overflow-y-auto p-6 md:p-8 border-r border-stone-200 space-y-5">
              <Section
                icon={<Server size={16} className="text-stone-600" />}
                title="Agent"
                items={{ [name ?? "agent"]: template.agent }}
              />
              <Section
                icon={<Brain size={16} className="text-purple-600" />}
                title="Models"
                items={template.models}
              />
              <Section
                icon={<Database size={16} className="text-blue-600" />}
                title="Knowledge"
                items={template.knowledge}
              />
              <Section
                icon={<Wrench size={16} className="text-orange-600" />}
                title="Tools"
                items={template.tools}
              />
              <Section
                icon={<Download size={16} className="text-teal-600" />}
                title="Ingestion"
                items={template.ingestion}
              />
              {template.interfaces && (
                <div>
                  <div className="flex items-center gap-2 mb-2">
                    <MessageSquare size={16} className="text-indigo-600" />
                    <span className="text-sm font-medium text-stone-700">Messaging</span>
                  </div>
                  <div className="bg-stone-50 border border-stone-200 p-3 space-y-0.5">
                    <KV label="Image" value={template.interfaces.image as string} />
                    <KV label="gRPC Port" value={template.interfaces.port as number} />
                    {"resources" in template.interfaces && (
                      <KV label="Resources" value={formatResources(template.interfaces.resources as Record<string, unknown>)} />
                    )}
                  </div>
                </div>
              )}
            </div>

            {/* Right — Config + Credentials */}
            <div className="overflow-y-auto p-6 md:p-8 space-y-6">
              <InterfacesPicker
                selected={selectedAdapters}
                onChange={setSelectedAdapters}
                adapterCredDefs={adapterCredDefs}
                adapterCredentials={adapterCredentials}
                onCredentialChange={setAdapterCredentials}
              />

              <ObservabilityToggle
                enabled={observabilityEnabled}
                onToggle={setObservabilityEnabled}
                config={template.observability as Record<string, unknown> | undefined}
              />

              <div>
                <div className="flex items-center gap-2 mb-2">
                  <Lock size={16} className="text-stone-600" />
                  <span className="text-sm font-medium text-stone-700">Credentials</span>
                </div>
                {variableEntries.length === 0 ? (
                  <p className="text-xs text-stone-500">No credentials required for this agent.</p>
                ) : (
                  <CredentialForm
                    variables={Object.fromEntries(variableEntries)}
                    values={credentialValues}
                    onChange={setCredentialValues}
                  />
                )}
              </div>
            </div>
          </div>
        )}

        {/* Errors / Validation result */}
        {(deployError || validationResult) && (
          <div className="shrink-0 px-6 md:px-8">
            {deployError && (
              <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm whitespace-pre-wrap">
                {deployError}
              </div>
            )}
            {validationResult && !deployError && (
              <div className={`p-3 border text-sm flex items-start gap-2 ${
                validationResult.valid
                  ? "bg-green-50 border-green-200 text-green-700"
                  : "bg-red-50 border-red-200 text-red-700"
              }`}>
                {validationResult.valid ? (
                  <>
                    <CheckCircle size={16} className="shrink-0 mt-0.5" />
                    <span>Validation passed. Ready to deploy.</span>
                  </>
                ) : (
                  <>
                    <AlertCircle size={16} className="shrink-0 mt-0.5" />
                    <div className="whitespace-pre-wrap">{validationResult.errors?.join("\n")}</div>
                  </>
                )}
              </div>
            )}
          </div>
        )}

        {/* Fixed footer — Validate + Deploy */}
        {template && !isLoading && (
          <div className="shrink-0 border-t border-stone-200 px-6 py-4 md:px-8 bg-white flex gap-3">
            <button
              type="button"
              onClick={handleValidate}
              disabled={!canDeploy || validateMutation.isPending}
              className="px-4 py-3 border border-stone-300 text-sm bg-white text-stone-700 hover:bg-stone-50 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
            >
              {validateMutation.isPending ? (
                <>
                  <Loader2 size={16} className="animate-spin" />
                  Validating...
                </>
              ) : (
                <>
                  <ShieldCheck size={16} />
                  Validate
                </>
              )}
            </button>
            <button
              type="submit"
              disabled={!canDeploy}
              className="flex-1 px-4 py-3 border border-stone-800 text-sm bg-stone-800 text-white hover:bg-stone-700 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
            >
              {deployMutation.isPending ? (
                <>
                  <Loader2 size={16} className="animate-spin" />
                  {isRedeploy ? "Updating..." : "Deploying..."}
                </>
              ) : (
                <>
                  {isRedeploy ? <RefreshCw size={16} /> : <Rocket size={16} />}
                  {isRedeploy ? "Update" : "Deploy"} {account}/{name}
                </>
              )}
            </button>
          </div>
        )}
      </form>
    </div>
  );
}
