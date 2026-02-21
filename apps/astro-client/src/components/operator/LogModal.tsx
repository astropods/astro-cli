import { useState, useCallback } from "react";
import { X, Loader2, RefreshCw } from "lucide-react";
import type { AgentDeployment, ApiError, PodDetail } from "../../lib/api";
import { useDeploymentLogs } from "../../api/queries/deployments";

export interface LogModalProps {
  accountName: string;
  deployment: AgentDeployment;
  pod: PodDetail;
  onClose: () => void;
}

export function LogModal({ accountName, deployment, pod, onClose }: LogModalProps) {
  const account = accountName;
  const [selectedContainer, setSelectedContainer] = useState(
    pod.containers[0]?.name || ""
  );
  const [tailLines, setTailLines] = useState(200);
  const logRef = useCallback((node: HTMLPreElement | null) => {
    if (node) node.scrollTop = node.scrollHeight;
  }, []);

  const { data: logs, isLoading: loading, error: logsError, refetch } = useDeploymentLogs(
    account, deployment.namespace, pod.name, selectedContainer, tailLines
  );
  const error = logsError
    ? (logsError as unknown as ApiError & { details?: string }).details
      ?? (logsError as unknown as ApiError).error_description
      ?? logsError.message
      ?? "Failed to fetch logs"
    : null;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-stone-300 w-full max-w-[800px] max-h-[85vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-stone-300">
          <div>
            <h2 className="text-lg font-semibold">Pod Logs</h2>
            <p className="text-sm text-stone-600 font-mono">{pod.name}</p>
          </div>
          <button
            className="bg-transparent border-none cursor-pointer p-1"
            onClick={onClose}
          >
            <X size={20} />
          </button>
        </div>

        <div className="flex items-center gap-3 px-4 py-2 border-b border-stone-200 bg-stone-50">
          {pod.containers.length > 1 && (
            <label className="flex items-center gap-1 text-sm">
              Container:
              <select
                value={selectedContainer}
                onChange={(e) => setSelectedContainer(e.target.value)}
                className="border border-stone-300 text-sm px-2 py-1"
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
              className="border border-stone-300 text-sm px-2 py-1"
            >
              {[50, 100, 200, 500].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
          <button
            onClick={() => refetch()}
            disabled={loading}
            className="flex items-center gap-1 px-2 py-1 text-sm border border-stone-300 bg-white hover:bg-stone-50 cursor-pointer disabled:opacity-50"
          >
            <RefreshCw size={14} className={loading ? "animate-spin" : ""} />
            Refresh
          </button>
        </div>

        <div className="flex-1 min-h-0 flex flex-col p-4">
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 size={24} className="animate-spin text-stone-500" />
              <span className="ml-2 text-stone-600">Loading logs...</span>
            </div>
          ) : error ? (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm">
              {error}
            </div>
          ) : (
            <pre
              ref={logRef}
              className="bg-stone-900 text-stone-100 text-xs font-mono p-3 flex-1 min-h-0 overflow-y-auto whitespace-pre-wrap break-all"
            >
              {logs ?? "(no logs available)"}
            </pre>
          )}
        </div>

        <div className="p-4 border-t border-stone-300">
          <button
            onClick={onClose}
            className="w-full px-4 py-2 border border-stone-800 text-sm bg-stone-800 text-white hover:bg-stone-700 cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
}
