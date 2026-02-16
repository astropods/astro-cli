import { useState } from "react";
import {
  RefreshCw,
  Loader2,
  ChevronDown,
  ChevronRight,
  Trash2,
  Activity,
  Link,
  Copy,
  FileText,
  Eye,
  History,
  Code,
} from "lucide-react";
import type { AgentDeployment, PodDetail } from "../../lib/api";
import { useRestartPod } from "../../api/queries/deployments";
import { LogModal } from "./LogModal";
import { DeploymentSpecModal } from "./DeploymentSpecModal";
import { DeploymentHistoryModal } from "./DeploymentHistoryModal";
import { EnvModal } from "./EnvModal";

function containerStateColor(state: string): string {
  switch (state) {
    case "Running":
      return "text-green-600";
    case "Waiting":
      return "text-yellow-600";
    case "Terminated":
      return "text-red-600";
    default:
      return "text-stone-500";
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
      return "text-stone-500";
  }
}

export interface DeploymentCardProps {
  accountName: string;
  deployment: AgentDeployment;
  onUndeploy: (name: string) => void;
  onRefresh: () => void;
  isUndeploying: boolean;
}

export function DeploymentCard({
  accountName,
  deployment,
  onUndeploy,
  onRefresh,
  isUndeploying,
}: DeploymentCardProps) {
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);
  const [logPod, setLogPod] = useState<PodDetail | null>(null);
  const [envPod, setEnvPod] = useState<PodDetail | null>(null);
  const [showSpec, setShowSpec] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const restartMutation = useRestartPod(accountName);

  const statusColor =
    deployment.status === "Running"
      ? "text-green-600"
      : deployment.status === "Pending"
        ? "text-yellow-600"
        : "text-stone-500";

  const statusBg =
    deployment.status === "Running"
      ? "bg-green-50 border-green-200"
      : deployment.status === "Pending"
        ? "bg-yellow-50 border-yellow-200"
        : "bg-stone-50 border-stone-200";

  const pods = deployment.pods || [];

  return (
    <div className="border border-stone-300 bg-white">
      <div
        className="flex items-center justify-between p-4 cursor-pointer hover:bg-stone-50"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-3">
          <Activity size={20} className={statusColor} />
          <div>
            <h3 className="font-semibold"><span className="font-normal text-stone-500">{accountName}/</span>{deployment.name}</h3>
            <p className="text-sm text-stone-500 font-mono">
              {deployment.build_id} &middot; {deployment.namespace}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span
            className={`px-2 py-1 text-xs border ${statusBg} ${statusColor}`}
          >
            {deployment.status}
          </span>
          <span className="text-sm text-stone-500">
            {deployment.ready}/{deployment.replicas} ready
          </span>
          <button
            onClick={(e) => {
              e.stopPropagation();
              setShowSpec(true);
            }}
            className="px-2 py-1.5 border border-stone-300 text-sm text-stone-600 bg-white hover:bg-stone-50 cursor-pointer flex items-center gap-1"
            title="View deployment spec"
          >
            <Eye size={14} />
            Spec
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              setShowHistory(true);
            }}
            className="px-2 py-1.5 border border-stone-300 text-sm text-stone-600 bg-white hover:bg-stone-50 cursor-pointer flex items-center gap-1"
            title="Deployment history"
          >
            <History size={14} />
            History
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onRefresh();
            }}
            className="px-2 py-1.5 border border-stone-300 text-sm text-stone-600 bg-white hover:bg-stone-50 cursor-pointer flex items-center gap-1"
            title="Refresh deployment status"
          >
            <RefreshCw size={14} />
            Refresh
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onUndeploy(deployment.name);
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
                className="px-2 py-0.5 text-xs bg-stone-100 border border-stone-200"
              >
                {c}
              </span>
            ))}
          </div>
        )}
        {deployment.external_urls && deployment.external_urls.length > 0 && (
          <div className="space-y-1 mb-2">
            {deployment.external_urls.map((ep) => (
              <div key={ep.url} className="flex items-center gap-2">
                <Link size={14} className="text-green-600 shrink-0" />
                <span className="text-xs text-stone-500 shrink-0">{ep.name}</span>
                <a
                  href={ep.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-xs text-blue-600 hover:underline font-mono truncate flex-1"
                  onClick={(e) => e.stopPropagation()}
                >
                  {ep.url}
                </a>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    navigator.clipboard.writeText(ep.url);
                    setCopiedUrl(ep.url);
                    setTimeout(() => setCopiedUrl(null), 2000);
                  }}
                  className="px-2 py-1 text-xs border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer flex items-center gap-1 shrink-0"
                  title="Copy endpoint URL"
                >
                  <Copy size={12} />
                  {copiedUrl === ep.url ? "Copied!" : "Copy"}
                </button>
              </div>
            ))}
          </div>
        )}
        <p className="text-xs text-stone-400">
          Deployed: {new Date(deployment.created_at).toLocaleString()}
        </p>
      </div>

      {/* Expanded pod details */}
      {expanded && (
        <div className="border-t border-stone-300 p-4 bg-stone-50">
          <h4 className="text-sm font-medium mb-3">
            Pods ({pods.length})
          </h4>
          {pods.length === 0 ? (
            <p className="text-sm text-stone-500">No pods found</p>
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
                    className="bg-white border border-stone-200 p-3"
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
                        <span className="text-xs text-stone-500">
                          {readyContainers}/{totalContainers} ready
                        </span>
                        {totalRestarts > 0 && (
                          <span className="text-xs text-orange-600">
                            {totalRestarts} restart{totalRestarts !== 1 ? "s" : ""}
                          </span>
                        )}
                        <span className="text-xs text-stone-400">{pod.age}</span>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setEnvPod(pod);
                          }}
                          className="px-2 py-1 text-xs border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer flex items-center gap-1"
                        >
                          <Code size={12} />
                          View Env
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setLogPod(pod);
                          }}
                          className="px-2 py-1 text-xs border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer flex items-center gap-1"
                        >
                          <FileText size={12} />
                          View Logs
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            if (confirm(`Restart pod ${pod.name}?`)) {
                              restartMutation.mutate({
                                namespace: deployment.namespace,
                                pod: pod.name,
                                account: accountName,
                              });
                            }
                          }}
                          disabled={restartMutation.isPending && restartMutation.variables?.pod === pod.name}
                          className="px-2 py-1 text-xs border border-orange-300 text-orange-600 bg-white hover:bg-orange-50 cursor-pointer disabled:opacity-50 flex items-center gap-1"
                        >
                          <RefreshCw size={12} className={restartMutation.isPending && restartMutation.variables?.pod === pod.name ? "animate-spin" : ""} />
                          Restart
                        </button>
                      </div>
                    </div>

                    {/* Container details */}
                    {pod.containers.length > 0 && (
                      <div className="mt-1">
                        {pod.containers.map((container) => (
                          <div
                            key={container.name}
                            className="flex items-center gap-3 text-xs py-1 border-t border-stone-100 first:border-t-0"
                          >
                            <span className="font-mono text-stone-700 w-32 truncate">
                              {container.name}
                            </span>
                            <span className={`font-medium ${containerStateColor(container.state)}`}>
                              {container.state}
                            </span>
                            {container.reason && (
                              <span className="text-stone-500" title={container.message || ""}>
                                ({container.reason})
                              </span>
                            )}
                            <span className={container.ready ? "text-green-600" : "text-stone-400"}>
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

      {showSpec && (
        <DeploymentSpecModal
          accountName={accountName}
          agentName={deployment.name}
          onClose={() => setShowSpec(false)}
        />
      )}

      {showHistory && (
        <DeploymentHistoryModal
          accountName={accountName}
          agentName={deployment.name}
          onClose={() => setShowHistory(false)}
        />
      )}

      {envPod && (
        <EnvModal
          accountName={accountName}
          deployment={deployment}
          pod={envPod}
          onClose={() => setEnvPod(null)}
        />
      )}
    </div>
  );
}
