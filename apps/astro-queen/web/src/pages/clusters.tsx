import { useState } from "react";
import {
  useClusters,
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
import { cn, formatDateTime } from "@/lib/utils";

function mutationErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return "Request failed";
}

export function ClustersPage() {
  const [enabledOnly, setEnabledOnly] = useState(false);
  const { data, isLoading, error } = useClusters(enabledOnly);
  const registerMut = useRegisterCluster();

  const [registerOpen, setRegisterOpen] = useState(false);
  const [id, setId] = useState("");
  const [region, setRegion] = useState("");
  const [eksName, setEksName] = useState("");
  const [eksEndpoint, setEksEndpoint] = useState("");

  const clusters = data?.clusters ?? [];

  const handleRegister = () => {
    registerMut.mutate(
      {
        id: id.trim(),
        region: region.trim(),
        eks_cluster_name: eksName.trim(),
        eks_cluster_endpoint: eksEndpoint.trim(),
        enabled: true,
      },
      {
        onSuccess: () => {
          setId("");
          setRegion("");
          setEksName("");
          setEksEndpoint("");
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
              !eksEndpoint.trim()
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
                <td colSpan={10} className="px-2 py-4 text-center text-muted-foreground">
                  No clusters found.
                </td>
              </tr>
            )}
            {clusters.map((cluster) => (
              <ClusterRow key={cluster.id} cluster={cluster} />
            ))}
          </tbody>
        </table>
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

function ClusterRow({ cluster }: { cluster: RegisteredCluster }) {
  const enableMut = useEnableCluster();
  const disableMut = useDisableCluster();
  const deregisterMut = useDeregisterCluster();
  const updateMut = useUpdateCluster();
  const healthMut = useCheckClusterHealth();

  const [editOpen, setEditOpen] = useState(false);
  const [region, setRegion] = useState(cluster.region);
  const [eksName, setEksName] = useState(cluster.eks_cluster_name);
  const [eksEndpoint, setEksEndpoint] = useState(cluster.eks_cluster_endpoint);

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
      },
      {
        onSuccess: () => setEditOpen(false),
      },
    );
  };

  const canSaveEdit =
    region.trim() !== "" && eksName.trim() !== "" && eksEndpoint.trim() !== "";

  return (
    <tr className="border-b border-glass-border-honey/50 hover:glass-subtle">
      <td className="px-2 py-1.5 font-mono">{cluster.id}</td>
      <td className="px-2 py-1.5">{cluster.region || "—"}</td>
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
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Edit cluster {cluster.id}</DialogTitle>
                  <DialogDescription>
                    Update region, EKS name, or endpoint. The cluster id cannot be changed.
                  </DialogDescription>
                </DialogHeader>
                <div className="grid gap-2">
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
