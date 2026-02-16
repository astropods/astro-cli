import { X, Loader2 } from "lucide-react";
import type { ApiError } from "../../lib/api";
import { useActiveDeploymentSpec } from "../../api/queries/deployments";

export interface DeploymentSpecModalProps {
  accountName: string;
  agentName: string;
  onClose: () => void;
}

export function DeploymentSpecModal({ accountName, agentName, onClose }: DeploymentSpecModalProps) {
  const { data, isLoading, error } = useActiveDeploymentSpec(accountName, agentName);

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-stone-300 w-full max-w-[700px] max-h-[85vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-stone-300">
          <div>
            <h2 className="text-lg font-semibold">
              <span className="font-normal text-stone-500">{accountName}/</span>
              {agentName}
            </h2>
            <p className="text-sm text-stone-600">Active deployment spec</p>
          </div>
          <button className="bg-transparent border-none cursor-pointer p-1" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4">
          {isLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 size={24} className="animate-spin text-stone-500" />
              <span className="ml-2 text-stone-600">Loading spec...</span>
            </div>
          ) : error ? (
            <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-sm">
              {(error as unknown as ApiError).error === "no active deployment found"
                ? "No active deployment found for this agent."
                : (error as unknown as ApiError).error ?? "Failed to load deployment spec"}
            </div>
          ) : data ? (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <span className="text-stone-500">Build</span>
                  <p className="font-mono">{data.build_id}</p>
                </div>
                <div>
                  <span className="text-stone-500">Namespace</span>
                  <p className="font-mono">{data.namespace}</p>
                </div>
                <div>
                  <span className="text-stone-500">Status</span>
                  <p className={data.status === "active" ? "text-green-600 font-medium" : "text-stone-600"}>
                    {data.status}
                  </p>
                </div>
                <div>
                  <span className="text-stone-500">Deployed at</span>
                  <p>{new Date(data.deployed_at).toLocaleString()}</p>
                </div>
              </div>

              <div>
                <h3 className="text-sm font-medium mb-2">Resolved Spec</h3>
                <pre className="bg-stone-900 text-stone-100 text-xs font-mono p-3 overflow-auto max-h-[400px] whitespace-pre-wrap break-all">
                  {JSON.stringify(data.spec, null, 2)}
                </pre>
              </div>
            </div>
          ) : null}
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
