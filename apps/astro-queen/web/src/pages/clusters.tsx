import { useState, type ClipboardEvent } from "react";
import {
  useClusters,
  useDeployments,
  useRegisterCluster,
  useEnableCluster,
  useDisableCluster,
  useDeregisterCluster,
  useUpdateCluster,
  useCheckClusterHealth,
  useRefreshMessagingCache,
} from "@/api/admin";
import type { RegisteredCluster } from "@/types/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogAction,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { ChevronDown, CircleCheck, CircleX, Copy, Pencil, Plus, RefreshCw } from "lucide-react";
import { cn, formatDateTime, countDeploymentsByRoutedCluster } from "@/lib/utils";

function mutationErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return "Request failed";
}

/** Accept AWS base64 or PEM; return base64 for JSON transport (Go []byte unmarshaling). */
function normalizeEksClusterCA(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return "";
  if (trimmed.includes("-----BEGIN")) {
    return btoa(trimmed);
  }
  return trimmed.replace(/\s+/g, "");
}

// ClusterDeployFields is the per-cluster deploy config astro-server requires
// on every additional cluster (ingress, Langfuse collector URL, netpol inputs).
// Both the register form and the edit dialog collect the same set.
type ClusterDeployFields = {
  agent_ingress_domain: string;
  ingestion_ingress_domain: string;
  knowledge_domain: string;
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
};

const emptyClusterDeploy: ClusterDeployFields = {
  agent_ingress_domain: "",
  ingestion_ingress_domain: "",
  knowledge_domain: "",
  langfuse_base_url_ext: "",
  langfuse_vpce_ips: "",
  pod_subnet_cidrs: "",
};

/** Primary cluster omits ingress fields in API JSON (env-sourced); coalesce before use. */
function clusterDeployFromCluster(cluster: Pick<RegisteredCluster, keyof ClusterDeployFields>): ClusterDeployFields {
  return {
    agent_ingress_domain: cluster.agent_ingress_domain ?? "",
    ingestion_ingress_domain: cluster.ingestion_ingress_domain ?? "",
    knowledge_domain: cluster.knowledge_domain ?? "",
    langfuse_base_url_ext: cluster.langfuse_base_url_ext ?? "",
    langfuse_vpce_ips: cluster.langfuse_vpce_ips ?? "",
    pod_subnet_cidrs: cluster.pod_subnet_cidrs ?? "",
  };
}

function trimClusterDeploy(f: ClusterDeployFields): ClusterDeployFields {
  const normalized = clusterDeployFromCluster(f);
  return {
    agent_ingress_domain: normalized.agent_ingress_domain.trim(),
    ingestion_ingress_domain: normalized.ingestion_ingress_domain.trim(),
    knowledge_domain: normalized.knowledge_domain.trim(),
    langfuse_base_url_ext: normalized.langfuse_base_url_ext.trim(),
    langfuse_vpce_ips: normalized.langfuse_vpce_ips.trim(),
    pod_subnet_cidrs: normalized.pod_subnet_cidrs.trim(),
  };
}

function clusterDeployComplete(f: ClusterDeployFields): boolean {
  const normalized = clusterDeployFromCluster(f);
  return (
    normalized.agent_ingress_domain.trim() !== "" &&
    normalized.ingestion_ingress_domain.trim() !== "" &&
    normalized.knowledge_domain.trim() !== "" &&
    normalized.langfuse_base_url_ext.trim() !== "" &&
    normalized.langfuse_vpce_ips.trim() !== "" &&
    normalized.pod_subnet_cidrs.trim() !== ""
  );
}

export function ClustersPage() {
  const [enabledOnly, setEnabledOnly] = useState(false);
  const { data, isLoading, error } = useClusters(enabledOnly);
  const { data: deploymentsData } = useDeployments();
  const registerMut = useRegisterCluster();

  const [registerOpen, setRegisterOpen] = useState(false);
  const [id, setId] = useState("");
  const [region, setRegion] = useState("");
  const [eksName, setEksName] = useState("");
  const [eksEndpoint, setEksEndpoint] = useState("");
  const [eksClusterCA, setEksClusterCA] = useState("");
  const [ingress, setIngress] = useState<ClusterDeployFields>(emptyClusterDeploy);

  const clusters = data?.clusters ?? [];
  const deploymentCounts = countDeploymentsByRoutedCluster(deploymentsData?.deployments ?? []);

  const handleRegister = () => {
    registerMut.mutate(
      {
        id: id.trim(),
        region: region.trim(),
        eks_cluster_name: eksName.trim(),
        eks_cluster_endpoint: eksEndpoint.trim(),
        eks_cluster_ca: normalizeEksClusterCA(eksClusterCA),
        enabled: true,
        ...trimClusterDeploy(ingress),
      },
      {
        onSuccess: () => {
          setId("");
          setRegion("");
          setEksName("");
          setEksEndpoint("");
          setEksClusterCA("");
          setIngress(emptyClusterDeploy);
          setRegisterOpen(false);
        },
      },
    );
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-xl font-semibold">Clusters</h2>
          <p className="text-[10px] text-muted-foreground">
            Primary cluster is configured via env vars. Register additional clusters for multi-region routing.
          </p>
        </div>
        <div className="flex items-start gap-3">
          <RefreshMessagingCacheButton />
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={enabledOnly}
              onChange={(e) => setEnabledOnly(e.target.checked)}
              className="rounded border-glass-border-honey"
            />
            Enabled only
          </label>
        </div>
      </div>

      <Collapsible open={registerOpen} onOpenChange={setRegisterOpen}>
        <CollapsibleTrigger asChild>
          <Button size="sm" variant="outline" className="gap-1">
            <Plus className="size-3.5" />
            Register cluster
            <ChevronDown
              className={cn("size-3.5 transition-transform", registerOpen && "rotate-180")}
            />
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent className="mt-3 rounded-lg glass p-3 space-y-3">
          <div className="grid gap-2 sm:grid-cols-2">
            <Field label="ID" value={id} onChange={setId} placeholder="us-west-2" />
            <Field label="Region" value={region} onChange={setRegion} placeholder="us-west-2" />
            <Field
              label="EKS cluster name"
              value={eksName}
              onChange={setEksName}
              placeholder="astro-preview-us-west-2"
            />
            <Field
              label="EKS endpoint"
              value={eksEndpoint}
              onChange={setEksEndpoint}
              placeholder="https://..."
            />
          </div>
          <EksClusterCAField value={eksClusterCA} onChange={setEksClusterCA} />
          <ClusterDeployFieldset value={ingress} onChange={setIngress} />
          {registerMut.isError && (
            <p className="text-destructive text-xs">{mutationErrorMessage(registerMut.error)}</p>
          )}
          <Button
            size="sm"
            onClick={handleRegister}
            disabled={
              registerMut.isPending ||
              !id.trim() ||
              !region.trim() ||
              !eksName.trim() ||
              !eksEndpoint.trim() ||
              !eksClusterCA.trim() ||
              !clusterDeployComplete(ingress)
            }
          >
            {registerMut.isPending ? "Registering…" : "Register"}
          </Button>
        </CollapsibleContent>
      </Collapsible>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full text-[11px] whitespace-nowrap">
          <thead>
            <tr className="border-b border-glass-border-honey glass-subtle">
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">ID</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Region</th>
              <th className="px-2 py-1.5 text-center font-medium text-muted-foreground">Deploys</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Agent domain</th>
              <th className="px-2 py-1.5 text-center font-medium text-muted-foreground">Ingress</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">EKS name</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Endpoint</th>
              <th className="px-2 py-1.5 text-center font-medium text-muted-foreground">Primary</th>
              <th className="px-2 py-1.5 text-center font-medium text-muted-foreground">Enabled</th>
              <th className="px-2 py-1.5 text-center font-medium text-muted-foreground">Healthy</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Health error</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Created</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Actions</th>
            </tr>
          </thead>
          <tbody>
            {clusters.length === 0 && !isLoading && (
              <tr>
                <td colSpan={13} className="px-2 py-4 text-center text-muted-foreground">
                  No clusters found.
                </td>
              </tr>
            )}
            {clusters.map((cluster) => (
              <ClusterRow
                key={cluster.id}
                cluster={cluster}
                deploymentCount={
                  cluster.is_primary
                    ? (deploymentCounts.get("primary") ?? 0)
                    : (deploymentCounts.get(cluster.id) ?? 0)
                }
              />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ClusterDeployFieldset({
  value,
  onChange,
}: {
  value: ClusterDeployFields;
  onChange: (v: ClusterDeployFields) => void;
}) {
  const set = (patch: Partial<ClusterDeployFields>) => onChange({ ...value, ...patch });
  return (
    <div className="space-y-2 rounded-md border border-glass-border-honey/60 p-2">
      <div className="text-xs font-medium text-muted-foreground">
        Ingress / knowledge
      </div>
      <p className="text-[10px] text-muted-foreground">
        Per-cluster overrides. All fields are required — astro-server rejects empty values for
        additional clusters; only the primary cluster reads env defaults. TLS and DNS for the
        tenant-router data plane are owned by the front-door ALB in astro-infra.
      </p>
      <div className="grid gap-2 sm:grid-cols-2">
        <Field
          label="Agent ingress domain"
          value={value.agent_ingress_domain}
          onChange={(v) => set({ agent_ingress_domain: v })}
          placeholder="agents.example.com"
        />
        <Field
          label="Ingestion ingress domain"
          value={value.ingestion_ingress_domain}
          onChange={(v) => set({ ingestion_ingress_domain: v })}
          placeholder="ingestion.example.com"
        />
        <Field
          label="Knowledge domain"
          value={value.knowledge_domain}
          onChange={(v) => set({ knowledge_domain: v })}
          placeholder="knowledge.example.com"
        />
      </div>
      <div className="text-xs font-medium text-muted-foreground pt-1">
        Langfuse / netpol (agent card sparklines)
      </div>
      <p className="text-[10px] text-muted-foreground">
        From managed-cluster infra outputs after Langfuse PrivateLink apply. VPCE IPs are bare
        addresses (no /32). Pod subnet CIDRs use standard notation (e.g. 10.0.0.0/24).
        Comma-separated lists.
      </p>
      <div className="grid gap-2 sm:grid-cols-2">
        <Field
          label="Langfuse base URL (collector)"
          value={value.langfuse_base_url_ext}
          onChange={(v) => set({ langfuse_base_url_ext: v })}
          placeholder="http://langfuse.platform.astroids.ai:3000"
        />
        <Field
          label="Langfuse VPCE IPs"
          value={value.langfuse_vpce_ips}
          onChange={(v) => set({ langfuse_vpce_ips: v })}
          placeholder="10.0.1.10,10.0.2.10"
        />
        <Field
          label="Pod subnet CIDRs"
          value={value.pod_subnet_cidrs}
          onChange={(v) => set({ pod_subnet_cidrs: v })}
          placeholder="10.0.0.0/24,10.1.0.0/24"
        />
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}) {
  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-7 text-xs"
      />
    </label>
  );
}

function EksClusterCAField({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  const handlePaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const text = e.clipboardData.getData("text");
    if (!text) return;
    e.preventDefault();
    onChange(normalizeEksClusterCA(text));
  };

  return (
    <label className="block space-y-1">
      <span className="text-xs text-muted-foreground">EKS cluster CA (base64 PEM)</span>
      <Textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onPaste={handlePaste}
        rows={3}
        placeholder="From `aws eks describe-cluster --query cluster.certificateAuthority.data` or byoc-output.sh"
        className="field-sizing-fixed max-h-32 min-h-16 resize-y overflow-y-auto break-all font-mono text-[10px]"
      />
      <p className="text-[10px] text-muted-foreground">
        Required for BYOC clusters. Paste the base64 value from byoc-output.sh or the PEM cert —
        both work. Capture once at registration; astro-server uses it instead of cross-account
        DescribeCluster.
      </p>
    </label>
  );
}

function ClusterRow({
  cluster,
  deploymentCount,
}: {
  cluster: RegisteredCluster;
  deploymentCount: number;
}) {
  const enableMut = useEnableCluster();
  const disableMut = useDisableCluster();
  const deregisterMut = useDeregisterCluster();
  const updateMut = useUpdateCluster();
  const healthMut = useCheckClusterHealth();

  const [editOpen, setEditOpen] = useState(false);
  const [region, setRegion] = useState(cluster.region);
  const [eksName, setEksName] = useState(cluster.eks_cluster_name);
  const [eksEndpoint, setEksEndpoint] = useState(cluster.eks_cluster_endpoint);
  const [eksClusterCA, setEksClusterCA] = useState(cluster.eks_cluster_ca ?? "");
  const [ingress, setIngress] = useState<ClusterDeployFields>(() => clusterDeployFromCluster(cluster));

  const busy =
    enableMut.isPending ||
    disableMut.isPending ||
    deregisterMut.isPending ||
    updateMut.isPending ||
    healthMut.isPending;
  const actionError =
    enableMut.error ??
    disableMut.error ??
    deregisterMut.error ??
    updateMut.error ??
    healthMut.error;

  const runEnable = () => enableMut.mutate(cluster.id);
  const runDisable = () => disableMut.mutate(cluster.id);
  const runDeregister = () => deregisterMut.mutate(cluster.id);
  const runHealthCheck = () => healthMut.mutate(cluster.id);

  const resetEditForm = () => {
    setRegion(cluster.region);
    setEksName(cluster.eks_cluster_name);
    setEksEndpoint(cluster.eks_cluster_endpoint);
    setEksClusterCA(cluster.eks_cluster_ca ?? "");
    setIngress(clusterDeployFromCluster(cluster));
  };

  const handleEditOpenChange = (open: boolean) => {
    setEditOpen(open);
    if (open) {
      resetEditForm();
    }
  };

  const handleSaveEdit = () => {
    updateMut.mutate(
      {
        id: cluster.id,
        region: region.trim(),
        eks_cluster_name: eksName.trim(),
        eks_cluster_endpoint: eksEndpoint.trim(),
        eks_cluster_ca: normalizeEksClusterCA(eksClusterCA),
        ...trimClusterDeploy(ingress),
      },
      {
        onSuccess: () => setEditOpen(false),
      },
    );
  };

  const canSaveEdit =
    region.trim() !== "" &&
    eksName.trim() !== "" &&
    eksEndpoint.trim() !== "" &&
    eksClusterCA.trim() !== "" &&
    clusterDeployComplete(ingress);

  const deployFields = clusterDeployFromCluster(cluster);
  const ingressOk = cluster.is_primary || clusterDeployComplete(deployFields);
  const agentDomain = deployFields.agent_ingress_domain || "—";

  return (
    <tr className="border-b border-glass-border-honey/50 hover:glass-subtle">
      <td className="px-2 py-1.5 font-mono">{cluster.id}</td>
      <td className="px-2 py-1.5">{cluster.region || "—"}</td>
      <td className="px-2 py-1.5 text-center tabular-nums">{deploymentCount}</td>
      <td
        className="max-w-[10rem] truncate px-2 py-1.5 font-mono text-[10px]"
        title={agentDomain}
      >
        {agentDomain}
      </td>
      <td className="px-2 py-1.5 text-center">
        {ingressOk ? (
          <span className="text-green-600">OK</span>
        ) : (
          <button
            type="button"
            className="rounded bg-amber-500/15 px-1.5 py-0.5 text-amber-800 hover:bg-amber-500/25"
            title="Ingress / cert / knowledge fields incomplete — edit cluster to fix"
            onClick={() => setEditOpen(true)}
          >
            Incomplete
          </button>
        )}
      </td>
      <td className="px-2 py-1.5">{cluster.eks_cluster_name || "—"}</td>
      <td
        className="max-w-[12rem] truncate px-2 py-1.5 font-mono"
        title={cluster.eks_cluster_endpoint}
      >
        {cluster.eks_cluster_endpoint || "—"}
      </td>
      <td className="px-2 py-1.5 text-center">
        {cluster.is_primary ? (
          <span className="rounded bg-blue-500/10 px-1.5 py-0.5 text-blue-600">Primary</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="px-2 py-1.5 text-center">
        {cluster.enabled ? (
          <span className="text-green-600">Yes</span>
        ) : (
          <span className="text-muted-foreground">No</span>
        )}
      </td>
      <td className="px-2 py-1.5 text-center">
        <div className="flex items-center justify-center gap-1">
          {cluster.healthy ? (
            <CircleCheck className="size-3.5 text-green-600" />
          ) : (
            <CircleX className="size-3.5 text-red-500" />
          )}
          <Button
            size="icon-xs"
            variant="ghost"
            disabled={busy}
            title="Retry health check"
            onClick={runHealthCheck}
          >
            <RefreshCw className={cn("size-3", healthMut.isPending && "animate-spin")} />
          </Button>
        </div>
      </td>
      <td className="max-w-md px-2 py-1.5 align-top whitespace-normal">
        {!cluster.healthy && cluster.health_error ? (
          <HealthErrorMessage message={cluster.health_error} />
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="px-2 py-1.5 text-muted-foreground">
        {cluster.created_at ? formatDateTime(cluster.created_at) : "—"}
      </td>
      <td className="px-2 py-1.5">
        {cluster.is_primary ? (
          <span className="text-[10px] text-muted-foreground">Env kubeconfig</span>
        ) : (
          <div className="flex flex-wrap items-center gap-1">
            <Dialog open={editOpen} onOpenChange={handleEditOpenChange}>
              <DialogTrigger asChild>
                <Button size="xs" variant="outline" disabled={busy}>
                  <Pencil className="size-3" />
                  Edit
                </Button>
              </DialogTrigger>
              <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
                <DialogHeader>
                  <DialogTitle>Edit cluster {cluster.id}</DialogTitle>
                  <DialogDescription>
                    Update EKS coordinates and the per-cluster ingress / cert / knowledge
                    config. The cluster id cannot be changed.
                  </DialogDescription>
                </DialogHeader>
                <div className="grid gap-2 sm:grid-cols-2">
                  <Field label="Region" value={region} onChange={setRegion} placeholder="us-west-2" />
                  <Field
                    label="EKS cluster name"
                    value={eksName}
                    onChange={setEksName}
                    placeholder="astro-preview-us-west-2"
                  />
                  <Field
                    label="EKS endpoint"
                    value={eksEndpoint}
                    onChange={setEksEndpoint}
                    placeholder="https://..."
                  />
                </div>
                <EksClusterCAField value={eksClusterCA} onChange={setEksClusterCA} />
                <ClusterDeployFieldset value={ingress} onChange={setIngress} />
                {updateMut.isError && (
                  <p className="text-destructive text-xs">{mutationErrorMessage(updateMut.error)}</p>
                )}
                <DialogFooter>
                  <Button variant="outline" onClick={() => setEditOpen(false)}>
                    Cancel
                  </Button>
                  <Button onClick={handleSaveEdit} disabled={updateMut.isPending || !canSaveEdit}>
                    {updateMut.isPending ? "Saving…" : "Save"}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
            {cluster.enabled ? (
              <Button size="xs" variant="outline" disabled={busy} onClick={runDisable}>
                Disable
              </Button>
            ) : (
              <Button size="xs" variant="outline" disabled={busy} onClick={runEnable}>
                Enable
              </Button>
            )}
            <DeregisterButton clusterId={cluster.id} disabled={busy} onConfirm={runDeregister} />
          </div>
        )}
        {actionError && (
          <p className="mt-1 max-w-xs text-[10px] text-destructive">
            {mutationErrorMessage(actionError)}
          </p>
        )}
      </td>
    </tr>
  );
}

function HealthErrorMessage({ message }: { message: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(message);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be unavailable; text remains selectable in the pre block.
    }
  };

  return (
    <div className="flex items-start gap-1.5">
      <pre className="min-w-0 flex-1 select-all whitespace-pre-wrap break-all rounded border border-red-500/20 bg-red-500/5 px-2 py-1.5 font-mono text-[10px] leading-relaxed text-red-600">
        {message}
      </pre>
      <Button
        size="xs"
        variant="outline"
        className="shrink-0"
        onClick={handleCopy}
        title="Copy error message"
      >
        <Copy className="size-3" />
        {copied ? "Copied" : "Copy"}
      </Button>
    </div>
  );
}

function RefreshMessagingCacheButton() {
  const refreshMut = useRefreshMessagingCache();
  return (
    <div className="flex flex-col items-end gap-1">
      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button
            size="sm"
            variant="outline"
            className="gap-1"
            disabled={refreshMut.isPending}
          >
            <RefreshCw className={cn("size-3.5", refreshMut.isPending && "animate-spin")} />
            {refreshMut.isPending ? "Refreshing…" : "Refresh messaging cache"}
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Refresh messaging cache?</AlertDialogTitle>
            <AlertDialogDescription>
              Evicts the messaging sidecar&apos;s ECR Docker Hub pull-through cache tag
              (astropods/messaging:latest) so the next agent pull re-imports it from
              Docker Hub, bypassing AWS&apos;s ~24h upstream-check window. Running agents keep
              their current sidecar until their pods restart or redeploy.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={() => refreshMut.mutate()}>
              Refresh
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      {refreshMut.isSuccess && (
        <p className="max-w-xs text-right text-[10px] text-green-600">
          {refreshMut.data?.message ?? "Cache refreshed."}
        </p>
      )}
      {refreshMut.isError && (
        <p className="max-w-xs text-right text-[10px] text-destructive">
          {mutationErrorMessage(refreshMut.error)}
        </p>
      )}
    </div>
  );
}

function DeregisterButton({
  clusterId,
  disabled,
  onConfirm,
}: {
  clusterId: string;
  disabled: boolean;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button size="xs" variant="destructive" disabled={disabled}>
          Deregister
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Deregister cluster {clusterId}?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the cluster from the registry. Deployments still referencing this cluster
            must be moved or deleted first.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Deregister
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
