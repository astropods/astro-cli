import { useState, type ClipboardEvent, type ReactNode } from "react";
import {
  useClusters,
  useDeployments,
  useRegisterCluster,
  useEnableCluster,
  useDisableCluster,
  useDeregisterCluster,
  useUpdateCluster,
  useCheckClusterHealth,
} from "@/api/admin";
import type { RegisteredCluster } from "@/types/admin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
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
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { CircleCheck, CircleX, Copy, Pencil, Plus, RefreshCw } from "lucide-react";
import { cn, formatDateTime, countDeploymentsByRoutedCluster, mutationErrorMessage } from "@/lib/utils";

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
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
};

const emptyClusterDeploy: ClusterDeployFields = {
  agent_ingress_domain: "",
  ingestion_ingress_domain: "",
  langfuse_base_url_ext: "",
  langfuse_vpce_ips: "",
  pod_subnet_cidrs: "",
};

/** Primary cluster omits ingress fields in API JSON (env-sourced); coalesce before use. */
function clusterDeployFromCluster(cluster: Pick<RegisteredCluster, keyof ClusterDeployFields>): ClusterDeployFields {
  return {
    agent_ingress_domain: cluster.agent_ingress_domain ?? "",
    ingestion_ingress_domain: cluster.ingestion_ingress_domain ?? "",
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
    normalized.langfuse_base_url_ext.trim() !== "" &&
    normalized.langfuse_vpce_ips.trim() !== "" &&
    normalized.pod_subnet_cidrs.trim() !== ""
  );
}

// ClusterFormValues backs both the register and edit dialogs — one form, one
// validation path, so the two flows can't drift out of sync.
type ClusterFormValues = {
  id: string;
  region: string;
  eksName: string;
  eksEndpoint: string;
  eksClusterCA: string;
  ingress: ClusterDeployFields;
};

function emptyClusterFormValues(): ClusterFormValues {
  return {
    id: "",
    region: "",
    eksName: "",
    eksEndpoint: "",
    eksClusterCA: "",
    ingress: emptyClusterDeploy,
  };
}

function clusterFormValuesFromCluster(cluster: RegisteredCluster): ClusterFormValues {
  return {
    id: cluster.id,
    region: cluster.region,
    eksName: cluster.eks_cluster_name,
    eksEndpoint: cluster.eks_cluster_endpoint,
    eksClusterCA: cluster.eks_cluster_ca ?? "",
    ingress: clusterDeployFromCluster(cluster),
  };
}

function clusterFormComplete(values: ClusterFormValues): boolean {
  return missingClusterFields(values).length === 0;
}

function missingClusterFields(values: ClusterFormValues): string[] {
  const missing: string[] = [];
  if (!values.id.trim()) missing.push("id");
  if (!values.region.trim()) missing.push("region");
  if (!values.eksName.trim()) missing.push("eks_cluster_name");
  if (!values.eksEndpoint.trim()) missing.push("eks_cluster_endpoint");
  if (!values.eksClusterCA.trim()) missing.push("eks_cluster_ca");
  if (!values.ingress.agent_ingress_domain.trim()) missing.push("agent_ingress_domain");
  if (!values.ingress.ingestion_ingress_domain.trim()) missing.push("ingestion_ingress_domain");
  if (!values.ingress.langfuse_base_url_ext.trim()) missing.push("langfuse_base_url_ext");
  if (!values.ingress.langfuse_vpce_ips.trim()) missing.push("langfuse_vpce_ips");
  if (!values.ingress.pod_subnet_cidrs.trim()) missing.push("pod_subnet_cidrs");
  return missing;
}

// ClusterApiPayload mirrors RegisterClusterRequest/UpdateClusterRequest field
// names exactly, so the JSON tab shows (and accepts) the real API body.
type ClusterApiPayload = {
  id: string;
  region: string;
  eks_cluster_name: string;
  eks_cluster_endpoint: string;
  eks_cluster_ca: string;
  agent_ingress_domain: string;
  ingestion_ingress_domain: string;
  langfuse_base_url_ext: string;
  langfuse_vpce_ips: string;
  pod_subnet_cidrs: string;
};

function valuesToApiPayload(values: ClusterFormValues): ClusterApiPayload {
  return {
    id: values.id.trim(),
    region: values.region.trim(),
    eks_cluster_name: values.eksName.trim(),
    eks_cluster_endpoint: values.eksEndpoint.trim(),
    eks_cluster_ca: normalizeEksClusterCA(values.eksClusterCA),
    ...trimClusterDeploy(values.ingress),
  };
}

function apiPayloadToValues(payload: Record<string, unknown>): ClusterFormValues {
  const str = (v: unknown) => (typeof v === "string" ? v : "");
  return {
    id: str(payload.id),
    region: str(payload.region),
    eksName: str(payload.eks_cluster_name),
    eksEndpoint: str(payload.eks_cluster_endpoint),
    eksClusterCA: str(payload.eks_cluster_ca),
    ingress: {
      agent_ingress_domain: str(payload.agent_ingress_domain),
      ingestion_ingress_domain: str(payload.ingestion_ingress_domain),
      langfuse_base_url_ext: str(payload.langfuse_base_url_ext),
      langfuse_vpce_ips: str(payload.langfuse_vpce_ips),
      pod_subnet_cidrs: str(payload.pod_subnet_cidrs),
    },
  };
}

function parseClusterJson(text: string): { ok: true; values: ClusterFormValues } | { ok: false; error: string } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : "Invalid JSON" };
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return { ok: false, error: "Expected a JSON object" };
  }
  return { ok: true, values: apiPayloadToValues(parsed as Record<string, unknown>) };
}

export function ClustersPage() {
  const [enabledOnly, setEnabledOnly] = useState(false);
  const { data, isLoading, error } = useClusters(enabledOnly);
  const { data: deploymentsData } = useDeployments();
  const registerMut = useRegisterCluster();

  const [registerOpen, setRegisterOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const clusters = data?.clusters ?? [];
  const deploymentCounts = countDeploymentsByRoutedCluster(deploymentsData?.deployments ?? []);
  const deployCountFor = (cluster: RegisteredCluster) =>
    cluster.is_primary ? (deploymentCounts.get("primary") ?? 0) : (deploymentCounts.get(cluster.id) ?? 0);
  const selectedCluster = clusters.find((c) => c.id === selectedId) ?? clusters[0] ?? null;

  const handleRegister = (values: ClusterFormValues) => {
    registerMut.mutate(
      { ...valuesToApiPayload(values), enabled: true },
      { onSuccess: () => setRegisterOpen(false) },
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

      <ClusterFormDialog
        open={registerOpen}
        onOpenChange={setRegisterOpen}
        trigger={
          <Button size="sm" variant="outline" className="gap-1">
            <Plus className="size-3.5" />
            Register cluster
          </Button>
        }
        title="Register cluster"
        description="Register an additional cluster for multi-region routing. All fields are required."
        idEditable
        initialValues={emptyClusterFormValues()}
        submitLabel="Register"
        pendingLabel="Registering…"
        isPending={registerMut.isPending}
        isError={registerMut.isError}
        error={registerMut.error}
        onSubmit={handleRegister}
      />

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      {!isLoading && !error && (
        <div className="flex gap-4">
          <div className="w-56 shrink-0 space-y-1">
            {clusters.length === 0 ? (
              <p className="rounded-lg glass px-3 py-4 text-center text-xs text-muted-foreground">
                No clusters found.
              </p>
            ) : (
              clusters.map((cluster) => (
                <ClusterListItem
                  key={cluster.id}
                  cluster={cluster}
                  deploymentCount={deployCountFor(cluster)}
                  selected={selectedCluster?.id === cluster.id}
                  onSelect={() => setSelectedId(cluster.id)}
                />
              ))
            )}
          </div>
          <div className="min-w-0 flex-1">
            {selectedCluster ? (
              <ClusterDetail cluster={selectedCluster} deploymentCount={deployCountFor(selectedCluster)} />
            ) : (
              <div className="flex h-40 items-center justify-center rounded-lg glass text-xs text-muted-foreground">
                Select a cluster to view details.
              </div>
            )}
          </div>
        </div>
      )}
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
        Ingress
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

function ClusterFormDialog({
  open,
  onOpenChange,
  trigger,
  title,
  description,
  idEditable,
  initialValues,
  submitLabel,
  pendingLabel,
  isPending,
  isError,
  error,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  trigger: ReactNode;
  title: string;
  description?: string;
  idEditable: boolean;
  initialValues: ClusterFormValues;
  submitLabel: string;
  pendingLabel: string;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  onSubmit: (values: ClusterFormValues) => void;
}) {
  const [values, setValues] = useState<ClusterFormValues>(initialValues);
  const [mode, setMode] = useState<"form" | "json">("form");
  const [jsonText, setJsonText] = useState("");
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [jsonValidated, setJsonValidated] = useState(false);
  const set = (patch: Partial<ClusterFormValues>) => setValues((v) => ({ ...v, ...patch }));

  const handleOpenChange = (next: boolean) => {
    if (next) {
      setValues(initialValues);
      setMode("form");
      setJsonError(null);
      setJsonValidated(false);
    }
    onOpenChange(next);
  };

  const handleTabChange = (next: string) => {
    if (next === "json") {
      setJsonText(JSON.stringify(valuesToApiPayload(values), null, 2));
      setJsonError(null);
      setJsonValidated(false);
      setMode("json");
      return;
    }
    // Switching back to the form: sync any edits made in the JSON tab first.
    const parsed = parseClusterJson(jsonText);
    if (!parsed.ok) {
      setJsonError(parsed.error);
      return;
    }
    setValues(parsed.values);
    setJsonError(null);
    setMode("form");
  };

  const handleValidateJson = () => {
    const parsed = parseClusterJson(jsonText);
    if (!parsed.ok) {
      setJsonError(parsed.error);
      setJsonValidated(false);
      return;
    }
    const missing = missingClusterFields(parsed.values);
    if (missing.length > 0) {
      setJsonError(`Missing required fields: ${missing.join(", ")}`);
      setJsonValidated(false);
      return;
    }
    setJsonError(null);
    setJsonValidated(true);
  };

  const handleSubmit = () => {
    if (mode === "json") {
      if (!jsonValidated) return;
      const parsed = parseClusterJson(jsonText);
      if (!parsed.ok) {
        setJsonError(parsed.error);
        setJsonValidated(false);
        return;
      }
      onSubmit(parsed.values);
      return;
    }
    onSubmit(values);
  };

  const submitDisabled = isPending || (mode === "form" ? !clusterFormComplete(values) : !jsonValidated);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        <Tabs value={mode} onValueChange={handleTabChange}>
          <TabsList>
            <TabsTrigger value="form">Form</TabsTrigger>
            <TabsTrigger value="json">JSON</TabsTrigger>
          </TabsList>
          <TabsContent value="form" className="space-y-3">
            <div className="grid gap-2 sm:grid-cols-2">
              {idEditable ? (
                <Field label="ID" value={values.id} onChange={(v) => set({ id: v })} placeholder="us-west-2" />
              ) : (
                <label className="block space-y-1">
                  <span className="text-xs text-muted-foreground">ID</span>
                  <Input value={values.id} disabled className="h-7 font-mono text-xs" />
                </label>
              )}
              <Field
                label="Region"
                value={values.region}
                onChange={(v) => set({ region: v })}
                placeholder="us-west-2"
              />
              <Field
                label="EKS cluster name"
                value={values.eksName}
                onChange={(v) => set({ eksName: v })}
                placeholder="astro-preview-us-west-2"
              />
              <Field
                label="EKS endpoint"
                value={values.eksEndpoint}
                onChange={(v) => set({ eksEndpoint: v })}
                placeholder="https://..."
              />
            </div>
            <EksClusterCAField value={values.eksClusterCA} onChange={(v) => set({ eksClusterCA: v })} />
            <ClusterDeployFieldset value={values.ingress} onChange={(v) => set({ ingress: v })} />
          </TabsContent>
          <TabsContent value="json" className="space-y-1.5">
            <p className="text-[10px] text-muted-foreground">
              Paste or edit the exact API request body. Field names match the REST endpoint.
            </p>
            <Textarea
              value={jsonText}
              onChange={(e) => {
                setJsonText(e.target.value);
                setJsonError(null);
                setJsonValidated(false);
              }}
              rows={16}
              className="field-sizing-fixed resize-y overflow-y-auto font-mono text-[10px]"
              spellCheck={false}
            />
            <div className="flex items-center gap-2">
              <Button size="xs" variant="outline" onClick={handleValidateJson}>
                Validate
              </Button>
              {jsonValidated && <span className="text-xs text-green-600">Valid — ready to save</span>}
            </div>
            {jsonError && <p className="text-destructive text-xs">{jsonError}</p>}
          </TabsContent>
        </Tabs>
        {isError && <p className="text-destructive text-xs">{mutationErrorMessage(error)}</p>}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={submitDisabled}>
            {isPending ? pendingLabel : submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ClusterListItem({
  cluster,
  deploymentCount,
  selected,
  onSelect,
}: {
  cluster: RegisteredCluster;
  deploymentCount: number;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        "w-full rounded-lg border px-3 py-2 text-left transition-colors",
        selected ? "glass-subtle border-glass-border-honey" : "border-transparent hover:glass-subtle",
      )}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-xs font-medium">{cluster.id}</span>
        {cluster.healthy ? (
          <CircleCheck className="size-3 shrink-0 text-green-600" />
        ) : (
          <CircleX className="size-3 shrink-0 text-red-500" />
        )}
      </div>
      <div className="mt-0.5 flex flex-wrap items-center gap-1 text-[10px] text-muted-foreground">
        <span>{cluster.region || "—"}</span>
        <span>·</span>
        <span>
          {deploymentCount} deploy{deploymentCount !== 1 ? "s" : ""}
        </span>
        {cluster.is_primary && (
          <span className="rounded bg-blue-500/10 px-1 text-blue-600">Primary</span>
        )}
        {!cluster.enabled && <span className="rounded bg-muted px-1">Disabled</span>}
      </div>
    </button>
  );
}

function DetailField({
  label,
  value,
  mono,
  small,
}: {
  label: string;
  value: string;
  mono?: boolean;
  small?: boolean;
}) {
  return (
    <div className="space-y-0.5">
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className={cn("break-all", mono && "font-mono", small ? "text-[10px]" : "text-xs")}>{value}</div>
    </div>
  );
}

function ClusterDetail({
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

  const handleSaveEdit = (values: ClusterFormValues) => {
    updateMut.mutate(
      { ...valuesToApiPayload(values), id: cluster.id },
      { onSuccess: () => setEditOpen(false) },
    );
  };

  const deployFields = clusterDeployFromCluster(cluster);
  const ingressOk = cluster.is_primary || clusterDeployComplete(deployFields);

  const [copied, setCopied] = useState(false);
  const handleCopyError = async () => {
    if (!cluster.health_error) return;
    try {
      await navigator.clipboard.writeText(cluster.health_error);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard may be unavailable; text remains selectable in the pre block.
    }
  };

  return (
    <div className="space-y-4 rounded-lg glass p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="font-mono text-base font-semibold">{cluster.id}</h3>
            {cluster.is_primary && (
              <span className="rounded bg-blue-500/10 px-1.5 py-0.5 text-[10px] text-blue-600">Primary</span>
            )}
            <span
              className={cn(
                "rounded px-1.5 py-0.5 text-[10px]",
                cluster.enabled ? "bg-green-500/10 text-green-600" : "bg-muted text-muted-foreground",
              )}
            >
              {cluster.enabled ? "Enabled" : "Disabled"}
            </span>
          </div>
          <p className="mt-0.5 text-[10px] text-muted-foreground">
            {deploymentCount} deployment{deploymentCount !== 1 ? "s" : ""} routed here
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <Button size="xs" variant="outline" disabled={busy} onClick={runHealthCheck} title="Retry health check">
            <RefreshCw className={cn("size-3", healthMut.isPending && "animate-spin")} />
            Check health
          </Button>
          {cluster.is_primary ? (
            <span className="text-[10px] text-muted-foreground">Env kubeconfig</span>
          ) : (
            <>
              <ClusterFormDialog
                open={editOpen}
                onOpenChange={setEditOpen}
                trigger={
                  <Button size="xs" variant="outline" disabled={busy}>
                    <Pencil className="size-3" />
                    Edit
                  </Button>
                }
                title={`Edit cluster ${cluster.id}`}
                description="Update EKS coordinates and the per-cluster ingress / cert config. The cluster id cannot be changed."
                idEditable={false}
                initialValues={clusterFormValuesFromCluster(cluster)}
                submitLabel="Save"
                pendingLabel="Saving…"
                isPending={updateMut.isPending}
                isError={updateMut.isError}
                error={updateMut.error}
                onSubmit={handleSaveEdit}
              />
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
            </>
          )}
        </div>
      </div>

      {actionError && (
        <p className="text-[10px] text-destructive">{mutationErrorMessage(actionError)}</p>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <DetailField label="Region" value={cluster.region || "—"} />
        <DetailField label="EKS cluster name" value={cluster.eks_cluster_name || "—"} />
        <DetailField label="EKS endpoint" value={cluster.eks_cluster_endpoint || "—"} mono />
        <DetailField label="Created" value={cluster.created_at ? formatDateTime(cluster.created_at) : "—"} />
      </div>

      <div className="space-y-1.5 rounded-md border border-glass-border-honey/60 p-3">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Ingress</span>
          {ingressOk ? (
            <span className="text-[10px] text-green-600">OK</span>
          ) : (
            <button
              type="button"
              className="rounded bg-amber-500/15 px-1.5 py-0.5 text-[10px] text-amber-800 hover:bg-amber-500/25"
              title="Ingress / cert fields incomplete — edit cluster to fix"
              onClick={() => setEditOpen(true)}
            >
              Incomplete
            </button>
          )}
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <DetailField label="Agent domain" value={deployFields.agent_ingress_domain || "—"} mono small />
          <DetailField label="Ingestion domain" value={deployFields.ingestion_ingress_domain || "—"} mono small />
        </div>
      </div>

      <div className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">Health</span>
        <div className="flex items-center gap-1.5 text-xs">
          {cluster.healthy ? (
            <>
              <CircleCheck className="size-3.5 text-green-600" />
              <span className="text-green-600">Healthy</span>
            </>
          ) : (
            <>
              <CircleX className="size-3.5 text-red-500" />
              <span className="text-red-500">Unhealthy</span>
            </>
          )}
        </div>
        {!cluster.healthy && cluster.health_error && (
          <div className="flex items-start gap-1.5">
            <pre className="min-w-0 flex-1 select-all whitespace-pre-wrap break-all rounded border border-red-500/20 bg-red-500/5 px-2 py-1.5 font-mono text-[10px] leading-relaxed text-red-600">
              {cluster.health_error}
            </pre>
            <Button size="xs" variant="outline" className="shrink-0" onClick={handleCopyError}>
              <Copy className="size-3" />
              {copied ? "Copied" : "Copy"}
            </Button>
          </div>
        )}
      </div>
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
