import { CheckCircle, AlertCircle, X } from "lucide-react";
import type { DeployResponse } from "../../lib/api";

export interface DeployResultModalProps {
  result: DeployResponse;
  onClose: () => void;
}

export function DeployResultModal({ result, onClose }: DeployResultModalProps) {
  const isSuccess = result.status === "success";
  const isPartial = result.status === "partial";

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-stone-300 w-full max-w-[500px] max-h-[80vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-stone-300">
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
              <p className="text-sm text-stone-600">
                <strong>Agent:</strong> {result.name}
              </p>
              <p className="text-sm text-stone-600">
                <strong>Build:</strong> {result.build_id}
              </p>
              <p className="text-sm text-stone-600">
                <strong>Namespace:</strong> {result.k8s_namespace}
              </p>
            </div>

            {result.resources && result.resources.length > 0 && (
              <div>
                <h3 className="text-sm font-medium mb-2">Deployed Resources</h3>
                <div className="bg-stone-50 p-2 border border-stone-200 text-xs font-mono max-h-32 overflow-y-auto">
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
                  <div className="bg-stone-50 p-2 border border-stone-200 text-xs font-mono">
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
