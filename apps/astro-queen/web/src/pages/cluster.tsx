import { useState } from "react";
import { useClusterStatus } from "@/api/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Search } from "lucide-react";

export function ClusterPage() {
  const [namespace, setNamespace] = useState("");
  const [activeNs, setActiveNs] = useState("");
  const { data, isLoading, error } = useClusterStatus(activeNs);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Cluster Status</h2>

      <div className="flex gap-2">
        <Input
          placeholder="Namespace..."
          value={namespace}
          onChange={(e) => setNamespace(e.target.value)}
          className="w-64"
          onKeyDown={(e) => { if (e.key === "Enter") setActiveNs(namespace); }}
        />
        <Button size="sm" onClick={() => setActiveNs(namespace)}>
          <Search className="size-3.5" />
          Query
        </Button>
      </div>

      {isLoading && <Skeleton className="h-64 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="space-y-6">
          {data.summary && (
            <div className="grid grid-cols-2 gap-3 md:grid-cols-5">
              <Stat label="Pods" value={data.summary.total_pods} />
              <Stat label="Running" value={data.summary.running_pods} />
              <Stat label="Pending" value={data.summary.pending_pods} />
              <Stat label="Failed" value={data.summary.failed_pods} warn={data.summary.failed_pods > 0} />
              <Stat label="Warning Events" value={data.summary.warning_events} warn={data.summary.warning_events > 0} />
            </div>
          )}

          {data.deployments?.length > 0 && (
            <Section title="Deployments">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                    <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Replicas</th>
                    <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Ready</th>
                    <th className="px-3 py-1.5 text-right font-medium text-muted-foreground">Available</th>
                  </tr>
                </thead>
                <tbody>
                  {data.deployments.map((d) => (
                    <tr key={d.name} className="border-b border-comb-light">
                      <td className="px-3 py-1.5">{d.name}</td>
                      <td className="px-3 py-1.5 text-right">{d.replicas}</td>
                      <td className="px-3 py-1.5 text-right">{d.ready_replicas}</td>
                      <td className="px-3 py-1.5 text-right">{d.available_replicas}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          )}

          {data.pods?.length > 0 && (
            <Section title="Pods">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Phase</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Node</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">IP</th>
                  </tr>
                </thead>
                <tbody>
                  {data.pods.map((p) => (
                    <tr key={p.name} className="border-b border-comb-light">
                      <td className="px-3 py-1.5 font-mono">{p.name}</td>
                      <td className={`px-3 py-1.5 ${p.phase === "Running" ? "text-green-600" : p.phase === "Pending" ? "text-yellow-600" : "text-destructive"}`}>
                        {p.phase}
                      </td>
                      <td className="px-3 py-1.5 text-muted-foreground">{p.node_name}</td>
                      <td className="px-3 py-1.5 text-muted-foreground">{p.pod_ip}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          )}

          {data.services?.length > 0 && (
            <Section title="Services">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Type</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Cluster IP</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Ports</th>
                  </tr>
                </thead>
                <tbody>
                  {data.services.map((svc) => (
                    <tr key={svc.name} className="border-b border-comb-light">
                      <td className="px-3 py-1.5">{svc.name}</td>
                      <td className="px-3 py-1.5 text-muted-foreground">{svc.type}</td>
                      <td className="px-3 py-1.5 text-muted-foreground">{svc.cluster_ip}</td>
                      <td className="px-3 py-1.5 text-muted-foreground">
                        {svc.ports?.map((p) => `${p.port}/${p.protocol}`).join(", ")}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          )}

          {data.ingresses?.length > 0 && (
            <Section title="Ingresses">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Name</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Hosts</th>
                    <th className="px-3 py-1.5 text-left font-medium text-muted-foreground">Class</th>
                  </tr>
                </thead>
                <tbody>
                  {data.ingresses.map((ing) => (
                    <tr key={ing.name} className="border-b border-comb-light">
                      <td className="px-3 py-1.5">{ing.name}</td>
                      <td className="px-3 py-1.5 text-muted-foreground">
                        {ing.rules?.map((r) => r.host).join(", ")}
                      </td>
                      <td className="px-3 py-1.5 text-muted-foreground">{ing.ingress_class_name}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Section>
          )}
        </div>
      )}
    </div>
  );
}

function Stat({ label, value, warn }: { label: string; value: number; warn?: boolean }) {
  return (
    <div className="rounded-lg glass px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`text-lg font-semibold ${warn ? "text-destructive" : ""}`}>{value}</p>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="mb-2 text-sm font-medium text-muted-foreground">{title}</h3>
      <div className="overflow-x-auto rounded-lg glass">{children}</div>
    </div>
  );
}
