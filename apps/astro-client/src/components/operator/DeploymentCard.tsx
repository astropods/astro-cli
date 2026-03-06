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
  Play,
} from "lucide-react";
import type { AgentDeployment, PodDetail } from "../../lib/api";
import { useRestartPod, useTriggerIngestion } from "../../api/queries/deployments";
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

function jobStatusBadge(status: string): { color: string; bg: string } {
  switch (status) {
    case "Running":
      return { color: "text-green-700", bg: "bg-green-50 border-green-200" };
    case "Succeeded":
      return { color: "text-blue-700", bg: "bg-blue-50 border-blue-200" };
    case "Failed":
      return { color: "text-red-700", bg: "bg-red-50 border-red-200" };
    default:
      return { color: "text-yellow-700", bg: "bg-yellow-50 border-yellow-200" };
  }
}

export interface DeploymentCardProps {
  accountName: string;
  deployment: AgentDeployment;
  onUndeploy?: (deploymentId: string) => void;
  onRefresh?: () => void;
  isUndeploying?: boolean;
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
  const triggerIngestion = useTriggerIngestion(accountName);

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
  const jobs = deployment.jobs || [];

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
          {onRefresh && (
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
          )}
          {onUndeploy && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onUndeploy(deployment.id ?? deployment.name);
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
          )}
          {expanded ? <ChevronDown size={20} /> : <ChevronRight size={20} />}
        </div>
      </div>

      {/* Summary info always visible */}
      <div className="px-4 pb-3">
        {(deployment.components.length > 0 || (deployment.manual_ingestions?.length ?? 0) > 0) && (
          <div className="flex gap-1 flex-wrap mb-2">
            {deployment.components.map((c) => (
              <span
                key={c}
                className="px-2 py-0.5 text-xs bg-stone-100 border border-stone-200 flex items-center gap-1"
              >
                {c}
              </span>
            ))}
            {deployment.manual_ingestions?.map((name) => {
              const isTriggeringThis = triggerIngestion.isPending && triggerIngestion.variables?.ingestion === name;
              return (
                <span
                  key={`manual-${name}`}
                  className="px-2 py-0.5 text-xs bg-stone-100 border border-stone-200 flex items-center gap-1"
                >
                  ingestion-{name}
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      triggerIngestion.mutate({
                        namespace: deployment.namespace,
                        ingestion: name,
                        account: accountName,
                      });
                    }}
                    disabled={isTriggeringThis}
                    className="ml-1 p-0.5 hover:bg-stone-200 cursor-pointer disabled:opacity-50 rounded-sm"
                    title={`Trigger ${name} ingestion`}
                  >
                    {isTriggeringThis ? (
                      <Loader2 size={12} className="animate-spin" />
                    ) : (
                      <Play size={12} className="text-stone-600" />
                    )}
                  </button>
                </span>
              );
            })}
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
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-2">
              {pods.map((pod) => {
                const readyContainers = pod.containers.filter((c) => c.ready).length;
                const totalContainers = pod.containers.length;
                const totalRestarts = pod.containers.reduce(
                  (sum, c) => sum + c.restart_count,
                  0
                );
                const isRestarting = restartMutation.isPending && restartMutation.variables?.pod === pod.name;

                return (
                  <div
                    key={pod.name}
                    className="bg-white border border-stone-200 p-2.5"
                  >
                    <div className="flex items-start justify-between gap-2 mb-1.5">
                      <div className="min-w-0">
                        <p className="font-mono text-xs truncate" title={pod.name}>
                          {pod.name}
                        </p>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className={`text-xs font-medium ${phaseColor(pod.phase)}`}>
                            {pod.phase}
                          </span>
                          <span className="text-xs text-stone-400">
                            {readyContainers}/{totalContainers} ready
                          </span>
                          {totalRestarts > 0 && (
                            <span className="text-xs text-orange-600">
                              {totalRestarts}↻
                            </span>
                          )}
                          <span className="text-xs text-stone-400">{pod.age}</span>
                        </div>
                      </div>
                      <div className="flex items-center gap-1 shrink-0">
                        <button
                          onClick={(e) => { e.stopPropagation(); setEnvPod(pod); }}
                          className="p-1 border border-stone-200 bg-white hover:bg-stone-50 cursor-pointer"
                          title="View Env"
                        >
                          <Code size={12} className="text-stone-500" />
                        </button>
                        <button
                          onClick={(e) => { e.stopPropagation(); setLogPod(pod); }}
                          className="p-1 border border-stone-200 bg-white hover:bg-stone-50 cursor-pointer"
                          title="View Logs"
                        >
                          <FileText size={12} className="text-stone-500" />
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
                          disabled={isRestarting}
                          className="p-1 border border-orange-200 bg-white hover:bg-orange-50 cursor-pointer disabled:opacity-50"
                          title="Restart Pod"
                        >
                          <RefreshCw size={12} className={`text-orange-500 ${isRestarting ? "animate-spin" : ""}`} />
                        </button>
                      </div>
                    </div>

                    {/* Container details */}
                    {pod.containers.length > 0 && (
                      <div className="border-t border-stone-100 pt-1.5 space-y-0.5">
                        {pod.containers.map((container) => (
                          <div
                            key={container.name}
                            className="flex items-center gap-2 text-xs"
                          >
                            <span className="font-mono text-stone-600 truncate max-w-[120px]" title={container.name}>
                              {container.name}
                            </span>
                            <span className={`font-medium ${containerStateColor(container.state)}`}>
                              {container.state}
                            </span>
                            {container.reason && (
                              <span className="text-stone-400 truncate" title={container.message || ""}>
                                {container.reason}
                              </span>
                            )}
                            {container.restart_count > 0 && (
                              <span className="text-orange-600">
                                {container.restart_count}↻
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

          {/* Job details */}
          {jobs.length > 0 && (
            <>
              <h4 className="text-sm font-medium mb-3 mt-4">
                Jobs ({jobs.length})
              </h4>
              <div className="space-y-2">
                {jobs.map((job) => {
                  const badge = jobStatusBadge(job.status);
                  return (
                    <div
                      key={job.name}
                      className="bg-white border border-stone-200 p-3 flex items-center justify-between"
                    >
                      <div className="flex items-center gap-3">
                        <span className="font-mono text-sm truncate max-w-[300px]">
                          {job.name}
                        </span>
                        <span className={`px-2 py-0.5 text-xs border ${badge.bg} ${badge.color}`}>
                          {job.status}
                        </span>
                        {job.component && (
                          <span className="text-xs text-stone-500">{job.component}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <span className="text-xs text-stone-500">
                          Completions: {job.completions}
                        </span>
                        <span className="text-xs text-stone-400">{job.age}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </>
          )}
        </div>
      )}

      {logPod && (
        <LogModal
          accountName={accountName}
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
