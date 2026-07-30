import { useState } from "react";
import { Link } from "react-router";
import {
  useAlerts,
  useClearAlert,
  useMuteAlert,
  useUnmuteAlert,
} from "@/api/admin";
import type { ActiveAlert, AlertCondition } from "@/types/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn, formatDateTime } from "@/lib/utils";
import { ChevronRight, BellOff } from "lucide-react";

const MUTE_DURATIONS: { label: string; seconds: number }[] = [
  { label: "1h", seconds: 3600 },
  { label: "6h", seconds: 6 * 3600 },
  { label: "24h", seconds: 24 * 3600 },
];

function severityClasses(sev?: string): string {
  switch (sev) {
    case "critical":
      return "bg-red-500/15 text-red-600 dark:text-red-400";
    case "warning":
      return "bg-amber-500/15 text-amber-600 dark:text-amber-400";
    default:
      return "bg-sky-500/15 text-sky-600 dark:text-sky-400";
  }
}

function stateClasses(state: string): string {
  switch (state) {
    case "firing":
      return "bg-red-500/15 text-red-600 dark:text-red-400";
    case "pending":
      return "bg-amber-500/15 text-amber-600 dark:text-amber-400";
    default:
      return "bg-muted text-muted-foreground";
  }
}

function Pill({ className, children }: { className: string; children: React.ReactNode }) {
  return (
    <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", className)}>
      {children}
    </span>
  );
}

export function AlertsPage() {
  const { data, isLoading, error } = useAlerts();
  const [search, setSearch] = useState("");

  const active = data?.active?.filter((a) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      (a.agent_name ?? "").toLowerCase().includes(q) ||
      (a.account_name ?? "").toLowerCase().includes(q) ||
      a.deployment_id.toLowerCase().includes(q) ||
      a.condition.toLowerCase().includes(q) ||
      (a.title ?? "").toLowerCase().includes(q) ||
      (a.workload ?? "").toLowerCase().includes(q)
    );
  });

  const firingCount = data?.active?.filter((a) => a.state === "firing").length ?? 0;
  const pendingCount = data?.active?.filter((a) => a.state === "pending").length ?? 0;
  const mutedCount = data?.active?.filter((a) => a.muted).length ?? 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Alerts</h2>
          <p className="text-[10px] text-muted-foreground">
            Observation alerts across all deployments.{" "}
            {data && (
              <>
                {firingCount} firing · {pendingCount} pending · {mutedCount} muted.
              </>
            )}
          </p>
        </div>
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search alerts..."
          className="w-56"
        />
      </div>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full text-[11px] whitespace-nowrap">
          <thead>
            <tr className="border-b border-glass-border-honey glass-subtle">
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Deployment</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Account</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Workload</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Condition</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Severity</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">State</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Since</th>
              <th className="px-2 py-1.5 text-right font-medium text-muted-foreground">Actions</th>
            </tr>
          </thead>
          <tbody>
            {active?.length === 0 && (
              <tr>
                <td colSpan={8} className="px-2 py-4 text-center text-muted-foreground">
                  {search ? "No matching alerts." : "No active alerts. All clear. 🎉"}
                </td>
              </tr>
            )}
            {active?.map((a) => (
              <AlertRow key={`${a.deployment_id}:${a.workload ?? ""}:${a.condition}`} alert={a} />
            ))}
          </tbody>
        </table>
      </div>

      <ConditionCatalog catalog={data?.catalog ?? []} />
    </div>
  );
}

function AlertRow({ alert: a }: { alert: ActiveAlert }) {
  const clear = useClearAlert();
  const mute = useMuteAlert();
  const unmute = useUnmuteAlert();
  const [muting, setMuting] = useState(false);

  const busy = clear.isPending || mute.isPending || unmute.isPending;

  return (
    <tr className="border-b border-glass-border-honey hover:bg-glass-light">
      <td className="px-2 py-1.5">
        <Link
          to={`/admin/deployments/${encodeURIComponent(a.deployment_id)}`}
          className="hover:underline"
          title={a.deployment_id}
        >
          {a.agent_name || a.deployment_id}
        </Link>
      </td>
      <td className="px-2 py-1.5 text-muted-foreground">
        {a.account_id ? (
          <Link to={`/admin/accounts/${encodeURIComponent(a.account_id)}`} className="hover:underline">
            {a.account_name || a.account_id}
          </Link>
        ) : (
          "—"
        )}
      </td>
      <td className="px-2 py-1.5 text-muted-foreground">{a.workload || "—"}</td>
      <td className="px-2 py-1.5" title={a.condition}>
        {a.title || a.condition}
      </td>
      <td className="px-2 py-1.5">
        <Pill className={severityClasses(a.severity)}>{a.severity || "—"}</Pill>
      </td>
      <td className="px-2 py-1.5">
        <div className="flex items-center gap-1">
          <Pill className={stateClasses(a.state)}>{a.state}</Pill>
          {a.muted && (
            <span
              className="inline-flex items-center gap-0.5 text-[10px] text-muted-foreground"
              title={a.muted_until ? `Muted until ${formatDateTime(a.muted_until)}` : "Muted"}
            >
              <BellOff className="size-2.5" />
              muted
            </span>
          )}
        </div>
      </td>
      <td className="px-2 py-1.5 text-muted-foreground">
        {a.active_since ? formatDateTime(a.active_since) : "—"}
      </td>
      <td className="px-2 py-1.5">
        <div className="flex items-center justify-end gap-1">
          {a.state !== "ok" && (
            <Button
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-[10px]"
              disabled={busy}
              onClick={() =>
                clear.mutate({
                  deployment_id: a.deployment_id,
                  workload: a.workload,
                  condition: a.condition,
                })
              }
            >
              Clear
            </Button>
          )}
          {a.muted ? (
            <Button
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-[10px]"
              disabled={busy}
              onClick={() =>
                unmute.mutate({ deployment_id: a.deployment_id, condition: a.condition })
              }
            >
              Unmute
            </Button>
          ) : muting ? (
            <div className="flex items-center gap-0.5">
              {MUTE_DURATIONS.map((d) => (
                <Button
                  key={d.seconds}
                  size="sm"
                  variant="ghost"
                  className="h-6 px-1.5 text-[10px]"
                  disabled={busy}
                  onClick={() => {
                    mute.mutate(
                      {
                        deployment_id: a.deployment_id,
                        condition: a.condition,
                        duration_seconds: d.seconds,
                      },
                      { onSettled: () => setMuting(false) },
                    );
                  }}
                >
                  {d.label}
                </Button>
              ))}
              <Button
                size="sm"
                variant="ghost"
                className="h-6 px-1.5 text-[10px] text-muted-foreground"
                onClick={() => setMuting(false)}
              >
                ✕
              </Button>
            </div>
          ) : (
            <Button
              size="sm"
              variant="ghost"
              className="h-6 px-2 text-[10px]"
              disabled={busy}
              onClick={() => setMuting(true)}
            >
              Mute
            </Button>
          )}
        </div>
      </td>
    </tr>
  );
}

function ConditionCatalog({ catalog }: { catalog: AlertCondition[] }) {
  const [open, setOpen] = useState(false);

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground">
        <ChevronRight className={cn("size-3.5 transition-transform", open && "rotate-90")} />
        Condition catalog ({catalog.length})
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 overflow-x-auto rounded-lg glass">
          <table className="w-full text-[11px]">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Condition</th>
                <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Severity</th>
                <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Description</th>
              </tr>
            </thead>
            <tbody>
              {catalog.map((c) => (
                <tr key={c.name} className="border-b border-glass-border-honey">
                  <td className="px-2 py-1.5" title={c.name}>
                    {c.title}
                  </td>
                  <td className="px-2 py-1.5">
                    <Pill className={severityClasses(c.severity)}>{c.severity}</Pill>
                  </td>
                  <td className="px-2 py-1.5 text-muted-foreground whitespace-normal">
                    {c.description}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
