import { useState } from "react";
import {
  X,
  Loader2,
  History,
  ChevronDown,
  ChevronRight,
  Clock,
} from "lucide-react";
import type { ApiError, DeploymentHistoryRecord } from "../../lib/api";
import { useDeploymentHistory } from "../../api/queries/deployments";

export interface DeploymentHistoryModalProps {
  accountName: string;
  agentName: string;
  onClose: () => void;
}

export function DeploymentHistoryModal({ accountName, agentName, onClose }: DeploymentHistoryModalProps) {
  const { data, isLoading, error } = useDeploymentHistory(accountName, agentName);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const deployments = data?.deployments ?? [];

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-stone-300 w-full max-w-[700px] max-h-[85vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-stone-300">
          <div>
            <h2 className="text-lg font-semibold">
              <span className="font-normal text-stone-500">{accountName}/</span>
              {agentName}
            </h2>
            <p className="text-sm text-stone-600">Deployment history</p>
          </div>
          <button className="bg-transparent border-none cursor-pointer p-1" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 size={24} className="animate-spin text-stone-500" />
              <span className="ml-2 text-stone-600">Loading history...</span>
            </div>
          ) : error ? (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm">
              {(error as unknown as ApiError).error ?? "Failed to load deployment history"}
            </div>
          ) : deployments.length === 0 ? (
            <div className="p-6 text-center">
              <History size={32} className="mx-auto text-stone-400 mb-2" />
              <p className="text-stone-600 text-sm">No deployment history found</p>
            </div>
          ) : (
            <div className="space-y-2">
              {deployments.map((d: DeploymentHistoryRecord) => (
                <HistoryEntry
                  key={d.id}
                  record={d}
                  isExpanded={expandedId === d.id}
                  onToggle={() => setExpandedId(expandedId === d.id ? null : d.id)}
                />
              ))}
            </div>
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

function HistoryEntry({
  record,
  isExpanded,
  onToggle,
}: {
  record: DeploymentHistoryRecord;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const isActive = record.status === "active";

  return (
    <div className="border border-stone-200">
      <div
        className="flex items-center justify-between p-3 cursor-pointer hover:bg-stone-50"
        onClick={onToggle}
      >
        <div className="flex items-center gap-3">
          <Clock size={16} className={isActive ? "text-green-600" : "text-stone-400"} />
          <div>
            <div className="flex items-center gap-2">
              <span className="font-mono text-sm">{record.build_id}</span>
              <span
                className={`px-1.5 py-0.5 text-xs border ${
                  isActive
                    ? "bg-green-50 border-green-200 text-green-700"
                    : "bg-stone-50 border-stone-200 text-stone-500"
                }`}
              >
                {record.status}
              </span>
            </div>
            <div className="text-xs text-stone-500 mt-0.5">
              Deployed: {new Date(record.deployed_at).toLocaleString()}
              {record.undeployed_at && (
                <span className="ml-2">
                  Undeployed: {new Date(record.undeployed_at).toLocaleString()}
                </span>
              )}
            </div>
          </div>
        </div>
        {isExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
      </div>

      {isExpanded && (
        <div className="border-t border-stone-200 p-3">
          <pre className="bg-stone-900 text-stone-100 text-xs font-mono p-3 overflow-auto max-h-[300px] whitespace-pre-wrap break-all">
            {JSON.stringify(record.spec, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}
