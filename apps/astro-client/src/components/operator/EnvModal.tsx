import { Fragment } from "react";
import { X, Loader2, Globe, Lock } from "lucide-react";
import type { AgentDeployment, PodDetail } from "../../lib/api";
import { useConfigMapData, useSecretKeys } from "../../api/queries/deployments";

export interface EnvModalProps {
  accountName: string;
  deployment: AgentDeployment;
  pod: PodDetail;
  onClose: () => void;
}

export function EnvModal({ accountName, deployment, pod, onClose }: EnvModalProps) {
  // Collect configmap and secret refs from all containers
  const allEnvVars = pod.containers.flatMap((c) => c.env ?? []);
  const configMapRefs = [...new Set(
    allEnvVars
      .filter((e) => e.from?.startsWith("configmap:"))
      .map((e) => e.from!.replace("configmap:", ""))
  )];
  const secretRefs = [...new Set(
    allEnvVars
      .filter((e) => e.from?.startsWith("secret:"))
      .map((e) => e.from!.replace("secret:", ""))
  )];

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-white border border-stone-300 w-full max-w-[700px] max-h-[85vh] relative overflow-hidden flex flex-col">
        <div className="flex items-center justify-between p-4 border-b border-stone-300">
          <div>
            <h2 className="text-lg font-semibold">Environment Variables</h2>
            <p className="text-sm text-stone-600 font-mono">{pod.name}</p>
          </div>
          <button className="bg-transparent border-none cursor-pointer p-1" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {/* Direct env vars per container */}
          {pod.containers.map((container) => {
            const directEnvVars = (container.env ?? []).filter(
              (e) => !e.from?.startsWith("configmap:") && !e.from?.startsWith("secret:")
            );
            if (directEnvVars.length === 0) return null;
            return (
              <div key={container.name}>
                <h3 className="text-sm font-medium mb-2">{container.name}</h3>
                <div className="bg-stone-50 border border-stone-200 p-3">
                  <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs font-mono">
                    {directEnvVars.map((ev) => (
                      <Fragment key={ev.name}>
                        <span className="text-stone-600">{ev.name}</span>
                        <span className="text-stone-800 truncate" title={ev.value ?? ev.from}>{ev.value ?? ev.from}</span>
                      </Fragment>
                    ))}
                  </div>
                </div>
              </div>
            );
          })}

          {/* ConfigMap data (resolved key-values) */}
          {configMapRefs.map((cmName) => (
            <ConfigMapSection
              key={cmName}
              accountName={accountName}
              namespace={deployment.namespace}
              configMapName={cmName}
            />
          ))}

          {/* Secret keys (names only, no values) */}
          {secretRefs.map((secretName) => (
            <SecretKeysSection
              key={secretName}
              accountName={accountName}
              namespace={deployment.namespace}
              secretName={secretName}
            />
          ))}
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

function ConfigMapSection({
  accountName,
  namespace,
  configMapName,
}: {
  accountName: string;
  namespace: string;
  configMapName: string;
}) {
  const { data, isLoading, error } = useConfigMapData(accountName, namespace, configMapName);

  return (
    <div>
      <h3 className="text-sm font-medium mb-2 flex items-center gap-1.5">
        <Globe size={14} className="text-stone-400" />
        ConfigMap: <span className="font-mono">{configMapName}</span>
      </h3>
      {isLoading ? (
        <div className="flex items-center gap-2 py-4 justify-center text-stone-500 text-sm">
          <Loader2 size={16} className="animate-spin" />
          Loading...
        </div>
      ) : error ? (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-xs">
          Failed to load ConfigMap data
        </div>
      ) : data?.data ? (
        <div className="bg-stone-50 border border-stone-200 p-3">
          <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs font-mono">
            {Object.entries(data.data).map(([key, value]) => (
              <Fragment key={key}>
                <span className="text-stone-600">{key}</span>
                <span className="text-stone-800 truncate" title={value}>{value}</span>
              </Fragment>
            ))}
          </div>
        </div>
      ) : (
        <div className="text-xs text-stone-500">No data</div>
      )}
    </div>
  );
}

function SecretKeysSection({
  accountName,
  namespace,
  secretName,
}: {
  accountName: string;
  namespace: string;
  secretName: string;
}) {
  const { data, isLoading, error } = useSecretKeys(accountName, namespace, secretName);

  return (
    <div>
      <h3 className="text-sm font-medium mb-2 flex items-center gap-1.5">
        <Lock size={14} className="text-stone-400" />
        Secret: <span className="font-mono">{secretName}</span>
      </h3>
      {isLoading ? (
        <div className="flex items-center gap-2 py-4 justify-center text-stone-500 text-sm">
          <Loader2 size={16} className="animate-spin" />
          Loading...
        </div>
      ) : error ? (
        <div className="p-3 bg-red-50 border border-red-200 text-red-700 text-xs">
          Failed to load secret keys
        </div>
      ) : data?.keys && data.keys.length > 0 ? (
        <div className="bg-stone-50 border border-stone-200 p-3">
          <div className="space-y-1 text-xs font-mono">
            {data.keys.map((key) => (
              <div key={key} className="flex items-center gap-2">
                <span className="text-stone-600">{key}</span>
                <span className="text-stone-400">--------</span>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className="text-xs text-stone-500">No keys</div>
      )}
    </div>
  );
}
