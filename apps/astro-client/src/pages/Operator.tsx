import { useState, useEffect, useCallback } from "react";
import {
  RefreshCw,
  Rocket,
  X,
  Loader2,
  Server,
  CheckCircle,
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Trash2,
  Activity,
  Link,
  Copy,
  FileText,
} from "lucide-react";
import { api } from "../lib/api";
import type {
  Agent,
  AgentDeployment,
  AgentsListResponse,
  CredentialInfo,
  DeployResponse,
  DeploymentsListResponse,
  PodDetail,
} from "../lib/api";
import { useAuth } from "../lib/auth";

interface DeployModalProps {
  agent: Agent;
  version: string;
  onClose: () => void;
  onDeploy: (credentials: Record<string, string>) => Promise<void>;
  isDeploying: boolean;
}

function DeployModal({
  agent,
  version,
  onClose,
  onDeploy,
  isDeploying,
}: DeployModalProps) {
  const [credentials, setCredentials] = useState<CredentialInfo[]>([]);
  const [credentialValues, setCredentialValues] = useState<
    Record<string, string>
  >({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchConfig() {
      try {
        setLoading(true);
        setError(null);
        const config = await api.getAgentConfig(agent.name, version);
        setCredentials(config.credentials || []);
        // Initialize credential values
        const initial: Record<string, string> = {};
        for (const cred of config.credentials || []) {
          initial[cred.key] = "";
        }
        setCredentialValues(initial);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Failed to load configuration"
        );
      } finally {
        setLoading(false);
      }
    }
    fetchConfig();
  }, [agent.name, version]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onDeploy(credentialValues);
  };

  const requiredCredentials = credentials.filter((c) => !c.optional);
  const optionalCredentials = credentials.filter((c) => c.optional);

  const canDeploy =
    !loading &&
    requiredCredentials.every((c) => credentialValues[c.key]?.trim());

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-gray-300 w-full max-w-[500px] max-h-[80vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-gray-300">
          <div>
            <h2 className="text-lg font-semibold">Deploy {agent.name}</h2>
            <p className="text-sm text-gray-600">Version: {version}</p>
          </div>
          <button
            className="bg-transparent border-none cursor-pointer p-1"
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 size={24} className="animate-spin text-gray-500" />
              <span className="ml-2 text-gray-600">Loading configuration...</span>
            </div>
          ) : error ? (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm">
              {error}
            </div>
          ) : credentials.length === 0 ? (
            <div className="p-3 bg-green-50 border border-green-200 text-green-700 text-sm">
              No credentials required for this agent.
            </div>
          ) : (
            <form onSubmit={handleSubmit} id="deploy-form">
              {requiredCredentials.length > 0 && (
                <div className="mb-4">
                  <h3 className="text-sm font-medium mb-2 text-gray-700">
                    Required Credentials
                  </h3>
                  <div className="space-y-3">
                    {requiredCredentials.map((cred) => (
                      <div key={cred.key}>
                        <label className="block text-sm font-medium text-gray-700 mb-1">
                          {cred.key}
                          <span className="ml-1 text-xs text-gray-500">
                            ({cred.provider})
                          </span>
                        </label>
                        <input
                          type="password"
                          value={credentialValues[cred.key] || ""}
                          onChange={(e) =>
                            setCredentialValues((prev) => ({
                              ...prev,
                              [cred.key]: e.target.value,
                            }))
                          }
                          placeholder={cred.description}
                          className="w-full py-2 px-3 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
                        />
                        <p className="text-xs text-gray-500 mt-1">
                          {cred.description}
                        </p>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {optionalCredentials.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium mb-2 text-gray-700">
                    Optional Credentials
                  </h3>
                  <div className="space-y-3">
                    {optionalCredentials.map((cred) => (
                      <div key={cred.key}>
                        <label className="block text-sm font-medium text-gray-700 mb-1">
                          {cred.key}
                          <span className="ml-1 text-xs text-gray-500">
                            ({cred.provider})
                          </span>
                        </label>
                        <input
                          type="password"
                          value={credentialValues[cred.key] || ""}
                          onChange={(e) =>
                            setCredentialValues((prev) => ({
                              ...prev,
                              [cred.key]: e.target.value,
                            }))
                          }
                          placeholder={cred.description}
                          className="w-full py-2 px-3 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
                        />
                        <p className="text-xs text-gray-500 mt-1">
                          {cred.description}
                        </p>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </form>
          )}
        </div>

        <div className="flex gap-2 p-4 border-t border-gray-300">
          <button
            type="button"
            onClick={onClose}
            className="flex-1 px-4 py-2 border border-gray-300 text-sm bg-white hover:bg-gray-50 cursor-pointer"
            disabled={isDeploying}
          >
            Cancel
          </button>
          <button
            type="submit"
            form="deploy-form"
            disabled={!canDeploy || isDeploying}
            className="flex-1 px-4 py-2 border border-gray-800 text-sm bg-gray-800 text-white hover:bg-gray-700 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
          >
            {isDeploying ? (
              <>
                <Loader2 size={16} className="animate-spin" />
                Deploying...
              </>
            ) : (
              <>
                <Rocket size={16} />
                Deploy
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

interface DeployResultModalProps {
  result: DeployResponse;
  onClose: () => void;
}

function DeployResultModal({ result, onClose }: DeployResultModalProps) {
  const isSuccess = result.status === "success";
  const isPartial = result.status === "partial";

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-gray-300 w-full max-w-[500px] max-h-[80vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-gray-300">
          <div className="flex items-center gap-2">
            {isSuccess ? (
              <CheckCircle size={20} className="text-green-600" />
            ) : isPartial ? (
              <AlertCircle size={20} className="text-yellow-600" />
            ) : (
              <AlertCircle size={20} className="text-red-600" />
            )}
            <h2 className="text-lg font-semibold">
              {isSuccess
                ? "Deployment Successful"
                : isPartial
                  ? "Deployment Partial"
                  : "Deployment Failed"}
            </h2>
          </div>
          <button
            className="bg-transparent border-none cursor-pointer p-1"
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          <div className="space-y-4">
            <div>
              <p className="text-sm text-gray-600">
                <strong>Agent:</strong> {result.name}
              </p>
              <p className="text-sm text-gray-600">
                <strong>Version:</strong> {result.version}
              </p>
              <p className="text-sm text-gray-600">
                <strong>Namespace:</strong> {result.k8s_namespace}
              </p>
            </div>

            {result.resources && result.resources.length > 0 && (
              <div>
                <h3 className="text-sm font-medium mb-2">Deployed Resources</h3>
                <div className="bg-gray-50 p-2 border border-gray-200 text-xs font-mono max-h-32 overflow-y-auto">
                  {result.resources.map((r, i) => (
                    <div key={i}>{r.kind}/{r.name}: {r.status}</div>
                  ))}
                </div>
              </div>
            )}

            {result.service_endpoints &&
              result.service_endpoints.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium mb-2">Service Endpoints</h3>
                  <div className="bg-gray-50 p-2 border border-gray-200 text-xs font-mono">
                    {result.service_endpoints.map((endpoint) => (
                      <div key={endpoint.name}>
                        {endpoint.name} ({endpoint.type}): {endpoint.url}
                      </div>
                    ))}
                  </div>
                </div>
              )}

            {result.errors && result.errors.length > 0 && (
              <div>
                <h3 className="text-sm font-medium mb-2 text-red-600">Errors</h3>
                <div className="bg-red-50 p-2 border border-red-200 text-xs text-red-700">
                  {result.errors.map((e, i) => (
                    <div key={i}>{e.kind}/{e.resource}: {e.error}</div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>

        <div className="p-4 border-t border-gray-300">
          <button
            onClick={onClose}
            className="w-full px-4 py-2 border border-gray-800 text-sm bg-gray-800 text-white hover:bg-gray-700 cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}

interface LogModalProps {
  deployment: AgentDeployment;
  pod: PodDetail;
  onClose: () => void;
}

function LogModal({ deployment, pod, onClose }: LogModalProps) {
  const [logs, setLogs] = useState<string>("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedContainer, setSelectedContainer] = useState(
    pod.containers[0]?.name || ""
  );
  const [tailLines, setTailLines] = useState(200);
  const logRef = useCallback((node: HTMLPreElement | null) => {
    if (node) node.scrollTop = node.scrollHeight;
  }, []);

  const fetchLogs = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const text = await api.getDeploymentLogs(
        deployment.name,
        deployment.version,
        pod.name,
        selectedContainer,
        tailLines
      );
      setLogs(text);
    } catch (err) {
      const apiErr = err as { error?: string; details?: string };
      setError(
        apiErr?.details || apiErr?.error || (err instanceof Error ? err.message : "Failed to fetch logs")
      );
    } finally {
      setLoading(false);
    }
  }, [deployment.name, deployment.version, pod.name, selectedContainer, tailLines]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-gray-300 w-full max-w-[800px] max-h-[85vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-gray-300">
          <div>
            <h2 className="text-lg font-semibold">Pod Logs</h2>
            <p className="text-sm text-gray-600 font-mono">{pod.name}</p>
          </div>
          <button
            className="bg-transparent border-none cursor-pointer p-1"
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </div>

        <div className="flex items-center gap-3 px-4 py-2 border-b border-gray-200 bg-gray-50">
          {pod.containers.length > 1 && (
            <label className="flex items-center gap-1 text-sm">
              Container:
              <select
                value={selectedContainer}
                onChange={(e) => setSelectedContainer(e.target.value)}
                className="border border-gray-300 text-sm px-2 py-1"
              >
                {pod.containers.map((c) => (
                  <option key={c.name} value={c.name}>
                    {c.name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="flex items-center gap-1 text-sm">
            Tail lines:
            <select
              value={tailLines}
              onChange={(e) => setTailLines(Number(e.target.value))}
              className="border border-gray-300 text-sm px-2 py-1"
            >
              {[50, 100, 200, 500].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <button
            onClick={fetchLogs}
            disabled={loading}
            className="flex items-center gap-1 px-2 py-1 text-sm border border-gray-300 bg-white hover:bg-gray-50 cursor-pointer disabled:opacity-50"
          >
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>

        <div className="flex-1 min-h-0 p-4">
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 size={24} className="animate-spin text-gray-500" />
              <span className="ml-2 text-gray-600">Loading logs...</span>
            </div>
          ) : error ? (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm">
              {error}
            </div>
          ) : (
            <pre
              ref={logRef}
              className="bg-gray-900 text-gray-100 text-xs font-mono p-3 overflow-y-scroll h-full whitespace-pre-wrap break-all"
            >
              {logs || "(no logs available)"}
            </pre>
          )}
        </div>

        <div className="p-4 border-t border-gray-300">
          <button
            onClick={onClose}
            className="w-full px-4 py-2 border border-gray-800 text-sm bg-gray-800 text-white hover:bg-gray-700 cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}

function containerStateColor(state: string): string {
  switch (state) {
    case "Running":
      return "text-green-600";
    case "Waiting":
      return "text-yellow-600";
    case "Terminated":
      return "text-red-600";
    default:
      return "text-gray-500";
  }
}

function phaseColor(phase: string): string {
  switch (phase) {
    case "Running":
      return "text-green-600";
    case "Pending":
      return "text-yellow-600";
    case "Failed":
      return "text-red-600";
    case "Succeeded":
      return "text-blue-600";
    default:
      return "text-gray-500";
  }
}

interface DeploymentCardProps {
  deployment: AgentDeployment;
  onUndeploy: (name: string, version: string) => void;
  isUndeploying: boolean;
}

function DeploymentCard({
  deployment,
  onUndeploy,
  isUndeploying,
}: DeploymentCardProps) {
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [logPod, setLogPod] = useState<PodDetail | null>(null);

  const statusColor =
    deployment.status === "Running"
      ? "text-green-600"
      : deployment.status === "Pending"
        ? "text-yellow-600"
        : "text-gray-500";

  const statusBg =
    deployment.status === "Running"
      ? "bg-green-50 border-green-200"
      : deployment.status === "Pending"
        ? "bg-yellow-50 border-yellow-200"
        : "bg-gray-50 border-gray-200";

  const handleCopyEndpoint = async () => {
    if (deployment.external_url) {
      await navigator.clipboard.writeText(deployment.external_url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const pods = deployment.pods || [];

  return (
    <div className="border border-gray-300 bg-white">
      <div
        className="flex items-center justify-between p-4 cursor-pointer hover:bg-gray-50"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-3">
          <Activity size={20} className={statusColor} />
          <div>
            <h3 className="font-semibold">{deployment.name}</h3>
            <p className="text-sm text-gray-500">
              Version: {deployment.version}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span
            className={`px-2 py-1 text-xs border ${statusBg} ${statusColor}`}
          >
            {deployment.status}
          </span>
          <span className="text-sm text-gray-500">
            {deployment.ready}/{deployment.replicas} ready
          </span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onUndeploy(deployment.name, deployment.version);
            }}
            disabled={isUndeploying}
            className="px-3 py-1.5 border border-red-300 text-sm text-red-600 bg-white hover:bg-red-50 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
          >
            {isUndeploying ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              <Trash2 size={14} />
            )}
            Undeploy
          </button>
          {expanded ? <ChevronDown size={20} /> : <ChevronRight size={20} />}
        </div>
      </div>

      {/* Summary info always visible */}
      <div className="px-4 pb-3">
        {deployment.components.length > 0 && (
          <div className="flex gap-1 flex-wrap mb-2">
            {deployment.components.map((c) => (
              <span
                key={c}
                className="px-2 py-0.5 text-xs bg-gray-100 border border-gray-200"
              >
                {c}
              </span>
            ))}
          </div>
        )}
        {deployment.external_url && (
          <div className="flex items-center gap-2 mb-2">
            <Link size={14} className="text-green-600" />
            <a
              href={deployment.external_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-blue-600 hover:underline font-mono truncate flex-1"
              onClick={(e) => e.stopPropagation()}
            >
              {deployment.external_url}
            </a>
            <button
              onClick={(e) => {
                e.stopPropagation();
                handleCopyEndpoint();
              }}
              className="px-2 py-1 text-xs border border-gray-300 bg-white hover:bg-gray-50 cursor-pointer flex items-center gap-1"
              title="Copy endpoint URL"
            >
              <Copy size={12} />
              {copied ? "Copied!" : "Copy"}
            </button>
          </div>
        )}
        <p className="text-xs text-gray-400">
          Deployed: {new Date(deployment.created_at).toLocaleString()}
        </p>
      </div>

      {/* Expanded pod details */}
      {expanded && (
        <div className="border-t border-gray-300 p-4 bg-gray-50">
          <h4 className="text-sm font-medium mb-3">
            Pods ({pods.length})
          </h4>
          {pods.length === 0 ? (
            <p className="text-sm text-gray-500">No pods found</p>
          ) : (
            <div className="space-y-2">
              {pods.map((pod) => {
                const readyContainers = pod.containers.filter((c) => c.ready).length;
                const totalContainers = pod.containers.length;
                const totalRestarts = pod.containers.reduce(
                  (sum, c) => sum + c.restart_count,
                  0
                );

                return (
                  <div
                    key={pod.name}
                    className="bg-white border border-gray-200 p-3"
                  >
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm truncate max-w-[300px]">
                          {pod.name}
                        </span>
                        <span className={`text-xs font-medium ${phaseColor(pod.phase)}`}>
                          {pod.phase}
                        </span>
                      </div>
                      <div className="flex items-center gap-3">
                        <span className="text-xs text-gray-500">
                          {readyContainers}/{totalContainers} ready
                        </span>
                        {totalRestarts > 0 && (
                          <span className="text-xs text-orange-600">
                            {totalRestarts} restart{totalRestarts !== 1 ? "s" : ""}
                          </span>
                        )}
                        <span className="text-xs text-gray-400">{pod.age}</span>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setLogPod(pod);
                          }}
                          className="px-2 py-1 text-xs border border-gray-300 bg-white hover:bg-gray-50 cursor-pointer flex items-center gap-1"
                        >
                          <FileText size={12} />
                          View Logs
                        </button>
                      </div>
                    </div>

                    {/* Container details */}
                    {pod.containers.length > 0 && (
                      <div className="mt-1">
                        {pod.containers.map((container) => (
                          <div
                            key={container.name}
                            className="flex items-center gap-3 text-xs py-1 border-t border-gray-100 first:border-t-0"
                          >
                            <span className="font-mono text-gray-700 w-32 truncate">
                              {container.name}
                            </span>
                            <span className={`font-medium ${containerStateColor(container.state)}`}>
                              {container.state}
                            </span>
                            {container.reason && (
                              <span className="text-gray-500" title={container.message || ""}>
                                ({container.reason})
                              </span>
                            )}
                            <span className={container.ready ? "text-green-600" : "text-gray-400"}>
                              {container.ready ? "Ready" : "Not Ready"}
                            </span>
                            {container.restart_count > 0 && (
                              <span className="text-orange-600">
                                {container.restart_count} restart{container.restart_count !== 1 ? "s" : ""}
                              </span>
                            )}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {logPod && (
        <LogModal
          deployment={deployment}
          pod={logPod}
          onClose={() => setLogPod(null)}
        />
      )}
    </div>
  );
}

interface AgentCardProps {
  agent: Agent;
  onDeploy: (agent: Agent, version: string) => void;
}

function AgentCard({ agent, onDeploy }: AgentCardProps) {
  const [expanded, setExpanded] = useState(false);
  const latestVersion = agent.versions[0];

  return (
    <div className="border border-gray-300 bg-white">
      <div
        className="flex items-center justify-between p-4 cursor-pointer hover:bg-gray-50"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-3">
          <Server size={20} className="text-gray-500" />
          <div>
            <h3 className="font-semibold">{agent.name}</h3>
            <p className="text-sm text-gray-500">
              {agent.versions.length} version(s) • Latest: {latestVersion?.version || "N/A"}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={(e) => {
              e.stopPropagation();
              if (latestVersion) {
                onDeploy(agent, latestVersion.version);
              }
            }}
            disabled={!latestVersion}
            className="px-3 py-1.5 border border-gray-800 text-sm bg-gray-800 text-white hover:bg-gray-700 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
          >
            <Rocket size={14} />
            Deploy
          </button>
          {expanded ? <ChevronDown size={20} /> : <ChevronRight size={20} />}
        </div>
      </div>

      {expanded && (
        <div className="border-t border-gray-300 p-4 bg-gray-50">
          <h4 className="text-sm font-medium mb-2">Available Versions</h4>
          <div className="space-y-2">
            {agent.versions.map((v) => (
              <div
                key={v.version}
                className="flex items-center justify-between p-2 bg-white border border-gray-200"
              >
                <div>
                  <span className="font-mono text-sm">{v.version}</span>
                  <span className="text-xs text-gray-500 ml-2">
                    Published: {new Date(v.published_at).toLocaleDateString()}
                  </span>
                </div>
                <button
                  onClick={() => onDeploy(agent, v.version)}
                  className="px-2 py-1 border border-gray-300 text-xs bg-white hover:bg-gray-50 cursor-pointer"
                >
                  Deploy this version
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export function Operator() {
  const { isAuthenticated, login } = useAuth();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [deployments, setDeployments] = useState<AgentDeployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [deploymentsLoading, setDeploymentsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Deploy modal state
  const [deployAgent, setDeployAgent] = useState<Agent | null>(null);
  const [deployVersion, setDeployVersion] = useState<string>("");
  const [isDeploying, setIsDeploying] = useState(false);
  const [deployResult, setDeployResult] = useState<DeployResponse | null>(null);

  // Undeploy state
  const [undeployingAgent, setUndeployingAgent] = useState<string | null>(null);

  const fetchAgents = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = (await api.listAgents()) as AgentsListResponse;
      setAgents(response.agents || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load agents");
    } finally {
      setLoading(false);
    }
  };

  const fetchDeployments = useCallback(async () => {
    if (!isAuthenticated) {
      setDeployments([]);
      return;
    }
    try {
      setDeploymentsLoading(true);
      const response = (await api.listDeployments()) as DeploymentsListResponse;
      setDeployments(response.deployments || []);
    } catch (err) {
      console.error("Failed to fetch deployments:", err);
      setDeployments([]);
    } finally {
      setDeploymentsLoading(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    fetchAgents();
  }, []);

  useEffect(() => {
    fetchDeployments();
  }, [fetchDeployments]);

  const handleDeploy = (agent: Agent, version: string) => {
    if (!isAuthenticated) {
      login();
      return;
    }
    setDeployAgent(agent);
    setDeployVersion(version);
  };

  const handleDeploySubmit = async (credentials: Record<string, string>) => {
    if (!deployAgent || !deployVersion) return;

    try {
      setIsDeploying(true);
      const result = await api.deployAgent({
        name: deployAgent.name,
        version: deployVersion,
        user_credentials: credentials,
      });
      setDeployResult(result);
      setDeployAgent(null);
      // Refresh deployments list
      fetchDeployments();
    } catch (err) {
      // Extract error details from API error response
      const apiErr = err as { error?: string; details?: string; validation_errors?: Array<{ field: string; message: string }>; missing_credentials?: string[] };
      const errors: Array<{ resource: string; kind: string; error: string }> = [];

      // Add validation errors
      if (apiErr.validation_errors?.length) {
        for (const ve of apiErr.validation_errors) {
          errors.push({
            resource: ve.field,
            kind: "Validation",
            error: ve.message,
          });
        }
      }

      // Add missing credentials
      if (apiErr.missing_credentials?.length) {
        for (const cred of apiErr.missing_credentials) {
          errors.push({
            resource: cred,
            kind: "Credential",
            error: `Missing required credential: ${cred}`,
          });
        }
      }

      // Fallback to generic error
      if (errors.length === 0) {
        errors.push({
          resource: "deploy",
          kind: "Request",
          error: apiErr.details || apiErr.error || (err instanceof Error ? err.message : "Deployment failed"),
        });
      }

      setDeployResult({
        status: "failed",
        name: deployAgent.name,
        version: deployVersion,
        k8s_namespace: "",
        deployed_at: new Date().toISOString(),
        resources: [],
        service_endpoints: [],
        errors,
      });
      setDeployAgent(null);
    } finally {
      setIsDeploying(false);
    }
  };

  const handleUndeploy = async (name: string, version: string) => {
    try {
      setUndeployingAgent(`${name}:${version}`);
      await api.undeployAgent({
        name,
        version,
      });
      // Refresh deployments list
      fetchDeployments();
    } catch (err) {
      console.error("Failed to undeploy:", err);
      alert(err instanceof Error ? err.message : "Failed to undeploy agent");
    } finally {
      setUndeployingAgent(null);
    }
  };

  return (
    <div className="max-w-[1000px]">
      <div className="flex justify-between items-start mb-6">
        <div>
          <h1 className="text-2xl font-semibold mb-1">Operator</h1>
          <p className="text-gray-600 text-sm">
            Manage and deploy agents from the registry
          </p>
        </div>
        <button
          onClick={fetchAgents}
          disabled={loading}
          className="flex items-center gap-2 px-4 py-2 border border-gray-300 bg-white text-sm text-gray-700 hover:bg-gray-50 cursor-pointer disabled:opacity-50"
        >
          <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          Refresh
        </button>
      </div>

      {!isAuthenticated && (
        <div className="mb-4 p-3 bg-yellow-50 border border-yellow-200 text-yellow-800 text-sm flex items-center gap-2">
          <AlertCircle size={16} />
          <span>
            You need to{" "}
            <button onClick={login} className="underline font-medium bg-transparent border-none cursor-pointer text-yellow-800">
              sign in
            </button>{" "}
            to deploy agents.
          </span>
        </div>
      )}

      {/* Current Deployments Section */}
      {isAuthenticated && (
        <div className="mb-8">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold">Current Deployments</h2>
            <button
              onClick={fetchDeployments}
              disabled={deploymentsLoading}
              className="flex items-center gap-1 px-2 py-1 text-sm text-gray-600 hover:text-gray-800"
            >
              <RefreshCw
                size={14}
                className={deploymentsLoading ? "animate-spin" : ""}
              />
              Refresh
            </button>
          </div>

          {deploymentsLoading ? (
            <div className="flex items-center justify-center py-8 border border-gray-300 bg-gray-50">
              <Loader2 size={24} className="animate-spin text-gray-500" />
            </div>
          ) : deployments.length === 0 ? (
            <div className="p-6 border border-gray-300 bg-gray-50 text-center">
              <Activity size={32} className="mx-auto text-gray-400 mb-2" />
              <p className="text-gray-600 text-sm">No active deployments</p>
              <p className="text-gray-500 text-xs mt-1">
                Deploy an agent from the registry below
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {deployments.map((dep) => (
                <DeploymentCard
                  key={`${dep.name}:${dep.version}`}
                  deployment={dep}
                  onUndeploy={handleUndeploy}
                  isUndeploying={undeployingAgent === `${dep.name}:${dep.version}`}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {/* Separator */}
      {isAuthenticated && (
        <div className="mb-6">
          <h2 className="text-lg font-semibold mb-1">Agent Registry</h2>
          <p className="text-gray-600 text-sm">
            Available agents to deploy
          </p>
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 size={32} className="animate-spin text-gray-500" />
        </div>
      ) : error ? (
        <div className="p-4 bg-red-50 border border-red-200 text-red-700">
          <p className="font-medium">Failed to load agents</p>
          <p className="text-sm">{error}</p>
          <button
            onClick={fetchAgents}
            className="mt-2 px-3 py-1 text-sm border border-red-300 bg-white text-red-700 hover:bg-red-50 cursor-pointer"
          >
            Retry
          </button>
        </div>
      ) : agents.length === 0 ? (
        <div className="p-8 border border-gray-300 text-center">
          <Server size={48} className="mx-auto text-gray-400 mb-4" />
          <h3 className="text-lg font-medium mb-2">No agents available</h3>
          <p className="text-gray-600 text-sm">
            There are no agents in the registry yet.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {agents.map((agent) => (
            <AgentCard key={agent.name} agent={agent} onDeploy={handleDeploy} />
          ))}
        </div>
      )}

      {deployAgent && (
        <DeployModal
          agent={deployAgent}
          version={deployVersion}
          onClose={() => setDeployAgent(null)}
          onDeploy={handleDeploySubmit}
          isDeploying={isDeploying}
        />
      )}

      {deployResult && (
        <DeployResultModal
          result={deployResult}
          onClose={() => setDeployResult(null)}
        />
      )}
    </div>
  );
}
