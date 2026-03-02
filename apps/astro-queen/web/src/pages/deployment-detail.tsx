import { useState } from "react";
import { useParams, useNavigate } from "react-router";
import { useDeployment, useDeleteDeployment, useRestartDeployment, usePodLogs, usePodEnv } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { ChevronDown, Trash2, RotateCw, FileText, Settings } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import type { K8sPodInfo } from "@/types/admin";

export function DeploymentDetailPage() {
  const { namespace } = useParams<{ namespace: string }>();
  const navigate = useNavigate();
  const { data, isLoading, error } = useDeployment(namespace ?? "");
  const deleteMut = useDeleteDeployment();
  const restartMut = useRestartDeployment();
  const [selectedPod, setSelectedPod] = useState<{ ns: string; name: string; mode: "logs" | "env" } | null>(null);

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-red-400">Error: {error.message}</p>;
  if (!data) return null;

  const { deployment: dep, cluster_status: cs } = data;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">{dep.name}</h2>
          <p className="text-sm text-zinc-500">{dep.namespace}</p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => restartMut.mutate(namespace!)}
            disabled={restartMut.isPending}
          >
            <RotateCw className="size-3.5" />
            Restart
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => {
              if (confirm("Delete this deployment?")) {
                deleteMut.mutate(namespace!, { onSuccess: () => navigate("/admin/deployments") });
              }
            }}
            disabled={deleteMut.isPending}
          >
            <Trash2 className="size-3.5" />
            Delete
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <InfoCard label="Status" value={dep.status} />
        <InfoCard label="Account" value={dep.account_name} />
        <InfoCard label="Build ID" value={dep.build_id} mono />
        <InfoCard label="Created" value={formatDateTime(dep.created_at)} />
      </div>

      {dep.components?.length > 0 && (
        <div>
          <h3 className="mb-2 text-sm font-medium text-zinc-400">Components</h3>
          <div className="flex flex-wrap gap-1.5">
            {dep.components.map((c) => (
              <span key={c} className="rounded bg-zinc-800 px-2 py-0.5 text-xs">{c}</span>
            ))}
          </div>
        </div>
      )}

      {cs && (
        <>
          {cs.summary && (
            <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
              <StatCard label="Pods" value={cs.summary.total_pods} sub={`${cs.summary.running_pods} running`} />
              <StatCard label="Deployments" value={cs.summary.total_deployments} />
              <StatCard label="Services" value={cs.summary.total_services} />
              <StatCard label="Ingresses" value={cs.summary.total_ingresses} />
              <StatCard label="Events" value={cs.summary.total_events} sub={cs.summary.warning_events > 0 ? `${cs.summary.warning_events} warnings` : undefined} warn={cs.summary.warning_events > 0} />
            </div>
          )}

          {cs.pods?.length > 0 && (
            <div>
              <h3 className="mb-2 text-sm font-medium text-zinc-400">Pods</h3>
              <div className="space-y-2">
                {cs.pods.map((pod) => (
                  <PodRow key={pod.name} pod={pod} namespace={namespace!} onSelect={setSelectedPod} />
                ))}
              </div>
            </div>
          )}

          {cs.events?.length > 0 && (
            <Collapsible>
              <CollapsibleTrigger className="flex items-center gap-1 text-sm font-medium text-zinc-400 hover:text-zinc-200">
                <ChevronDown className="size-4" />
                Events ({cs.events.length})
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-2">
                <div className="overflow-x-auto rounded-md border border-zinc-800">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-zinc-800 bg-zinc-900/50">
                        <th className="px-3 py-1.5 text-left font-medium text-zinc-400">Type</th>
                        <th className="px-3 py-1.5 text-left font-medium text-zinc-400">Reason</th>
                        <th className="px-3 py-1.5 text-left font-medium text-zinc-400">Message</th>
                        <th className="px-3 py-1.5 text-left font-medium text-zinc-400">Object</th>
                        <th className="px-3 py-1.5 text-right font-medium text-zinc-400">Count</th>
                      </tr>
                    </thead>
                    <tbody>
                      {cs.events.map((ev, i) => (
                        <tr key={i} className="border-b border-zinc-800/50">
                          <td className={`px-3 py-1.5 ${ev.type === "Warning" ? "text-yellow-400" : "text-zinc-400"}`}>{ev.type}</td>
                          <td className="px-3 py-1.5">{ev.reason}</td>
                          <td className="max-w-md truncate px-3 py-1.5 text-zinc-400">{ev.message}</td>
                          <td className="px-3 py-1.5 text-zinc-500">{ev.involved_object}</td>
                          <td className="px-3 py-1.5 text-right text-zinc-500">{ev.count}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </CollapsibleContent>
            </Collapsible>
          )}
        </>
      )}

      {selectedPod && (
        <PodDetail
          namespace={selectedPod.ns}
          pod={selectedPod.name}
          mode={selectedPod.mode}
          onClose={() => setSelectedPod(null)}
        />
      )}
    </div>
  );
}

function InfoCard({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-md border border-zinc-800 bg-zinc-900/30 px-3 py-2">
      <p className="text-xs text-zinc-500">{label}</p>
      <p className={`mt-0.5 truncate text-sm ${mono ? "font-mono text-xs" : ""}`}>{value || "-"}</p>
    </div>
  );
}

function StatCard({ label, value, sub, warn }: { label: string; value: number; sub?: string; warn?: boolean }) {
  return (
    <div className="rounded-md border border-zinc-800 bg-zinc-900/30 px-3 py-2">
      <p className="text-xs text-zinc-500">{label}</p>
      <p className="text-lg font-semibold">{value}</p>
      {sub && <p className={`text-xs ${warn ? "text-yellow-400" : "text-zinc-500"}`}>{sub}</p>}
    </div>
  );
}

function PodRow({
  pod,
  namespace,
  onSelect,
}: {
  pod: K8sPodInfo;
  namespace: string;
  onSelect: (sel: { ns: string; name: string; mode: "logs" | "env" }) => void;
}) {
  return (
    <div className="flex items-center justify-between rounded-md border border-zinc-800 bg-zinc-900/30 px-3 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">{pod.name}</p>
        <p className="text-xs text-zinc-500">
          {pod.phase} &middot; {pod.node_name} &middot; {pod.pod_ip}
        </p>
      </div>
      <div className="flex shrink-0 gap-1">
        <Button variant="ghost" size="icon-xs" onClick={() => onSelect({ ns: namespace, name: pod.name, mode: "logs" })}>
          <FileText className="size-3.5" />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={() => onSelect({ ns: namespace, name: pod.name, mode: "env" })}>
          <Settings className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

function PodDetail({ namespace, pod, mode, onClose }: { namespace: string; pod: string; mode: "logs" | "env"; onClose: () => void }) {
  const logsQuery = usePodLogs(mode === "logs" ? namespace : "", mode === "logs" ? pod : "");
  const envQuery = usePodEnv(mode === "env" ? namespace : "", mode === "env" ? pod : "");

  return (
    <div className="rounded-md border border-zinc-800 bg-zinc-900/50 p-4">
      <div className="mb-3 flex items-center justify-between">
        <h4 className="font-medium">{pod} - {mode === "logs" ? "Logs" : "Environment"}</h4>
        <Button variant="ghost" size="xs" onClick={onClose}>Close</Button>
      </div>
      {mode === "logs" && (
        logsQuery.isLoading ? <Skeleton className="h-40 w-full" /> :
        logsQuery.error ? <p className="text-red-400 text-sm">{logsQuery.error.message}</p> :
        <pre className="max-h-96 overflow-auto rounded bg-zinc-950 p-3 text-xs text-zinc-300">{logsQuery.data?.logs || "No logs"}</pre>
      )}
      {mode === "env" && (
        envQuery.isLoading ? <Skeleton className="h-40 w-full" /> :
        envQuery.error ? <p className="text-red-400 text-sm">{envQuery.error.message}</p> :
        <div className="space-y-3">
          {envQuery.data?.containers?.map((c) => (
            <div key={c.container}>
              <p className="mb-1 text-xs font-medium text-zinc-400">{c.container}</p>
              <div className="overflow-x-auto rounded bg-zinc-950">
                <table className="w-full text-xs">
                  <tbody>
                    {c.vars?.map((v) => (
                      <tr key={v.name} className="border-b border-zinc-900">
                        <td className="whitespace-nowrap px-2 py-1 font-mono text-amber">{v.name}</td>
                        <td className="px-2 py-1 font-mono text-zinc-400">{v.value || v.value_from || "-"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
