import { useState } from "react";
import {
  useEvaluators,
  useRunEvaluatorSweep,
  useEvaluatorDrift,
  useFixDeploymentDrift,
} from "@/api/admin";
import { Button } from "@/components/ui/button";
import { RefreshCw, ChevronDown, ChevronRight, Wrench, Hammer } from "lucide-react";
import { cn, mutationErrorMessage } from "@/lib/utils";
import { formatDistanceToNow } from "date-fns";
import type { EvaluatorSummary, EvaluatorDriftRow } from "@/types/admin";

// Evaluators (astro-server's internal/deployeval) are named, declarative
// checks for one kind of deployment configuration drift, each paired with a
// fix. An operator runs a check on demand, sees which deployments drifted,
// and fixes them individually. New evaluators need no frontend change — this
// panel renders whatever ListEvaluators returns.
export function EvaluatorsPanel() {
  const { data, isLoading } = useEvaluators();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const evaluators = data?.evaluators ?? [];

  if (isLoading || evaluators.length === 0) return null;

  return (
    <div className="mt-6">
      <h3 className="mb-2 text-sm font-semibold">Evaluators</h3>
      <div className="space-y-2">
        {evaluators.map((ev) => (
          <EvaluatorRow
            key={ev.id}
            evaluator={ev}
            expanded={expandedId === ev.id}
            onToggle={() =>
              setExpandedId((cur) => (cur === ev.id ? null : ev.id))
            }
          />
        ))}
      </div>
    </div>
  );
}

function EvaluatorRow({
  evaluator,
  expanded,
  onToggle,
}: {
  evaluator: EvaluatorSummary;
  expanded: boolean;
  onToggle: () => void;
}) {
  const runMut = useRunEvaluatorSweep();

  return (
    <div className="rounded-lg glass px-3 py-2">
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={onToggle}
          className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
        >
          {expanded ? (
            <ChevronDown className="size-3.5 shrink-0" />
          ) : (
            <ChevronRight className="size-3.5 shrink-0" />
          )}
          <span className="truncate text-[13px] font-medium">
            {evaluator.name}
          </span>
          {!!evaluator.drifted_count && (
            <span className="shrink-0 rounded-full bg-amber-100/60 px-1.5 py-0.5 text-[10px] text-amber-800">
              {evaluator.drifted_count} drifted
            </span>
          )}
          {!!evaluator.fix_failed_count && (
            <span className="shrink-0 rounded-full bg-red-100/60 px-1.5 py-0.5 text-[10px] text-red-700">
              {evaluator.fix_failed_count} fix failed
            </span>
          )}
        </button>
        <span className="shrink-0 text-[10px] text-muted-foreground">
          {evaluator.last_checked_at
            ? `checked ${formatDistanceToNow(new Date(evaluator.last_checked_at), { addSuffix: true })}`
            : "never checked"}
        </span>
        <Button
          size="xs"
          variant="outline"
          className="shrink-0 gap-1"
          disabled={runMut.isPending}
          onClick={() => runMut.mutate(evaluator.id)}
        >
          <RefreshCw
            className={cn("size-3", runMut.isPending && "animate-spin")}
          />
          {runMut.isPending ? "Checking…" : "Run check"}
        </Button>
      </div>
      <p className="mt-1 text-[11px] text-muted-foreground">
        {evaluator.description}
      </p>
      {runMut.isError && (
        <p className="mt-1 text-[10px] text-destructive">
          {mutationErrorMessage(runMut.error)}
        </p>
      )}
      {expanded && <EvaluatorDriftList evaluatorId={evaluator.id} />}
    </div>
  );
}

function EvaluatorDriftList({ evaluatorId }: { evaluatorId: string }) {
  const { data, isLoading } = useEvaluatorDrift(evaluatorId);
  const fixMut = useFixDeploymentDrift();
  const rows = data?.rows ?? [];

  const [fixingAll, setFixingAll] = useState(false);
  const [progress, setProgress] = useState(0);

  const fixAll = async () => {
    setFixingAll(true);
    setProgress(0);
    for (let i = 0; i < rows.length; i++) {
      try {
        await fixMut.mutateAsync({
          evaluatorId,
          deploymentId: rows[i].deployment_id,
        });
      } catch {
        // continue to the next deployment; per-row status still reflects
        // whatever FixDeploymentDrift last recorded for the failed one
      }
      setProgress(i + 1);
    }
    setFixingAll(false);
  };

  if (isLoading) {
    return (
      <p className="mt-2 text-[11px] text-muted-foreground">Loading…</p>
    );
  }
  if (rows.length === 0) {
    return (
      <p className="mt-2 text-[11px] text-muted-foreground">
        No drifted deployments.
      </p>
    );
  }

  return (
    <>
      <div className="mt-2 flex justify-end">
        <Button
          size="xs"
          variant="outline"
          className="gap-1"
          disabled={fixingAll}
          onClick={fixAll}
        >
          <Hammer className={cn("size-3", fixingAll && "animate-pulse")} />
          {fixingAll ? `Fixing… ${progress}/${rows.length}` : "Fix all"}
        </Button>
      </div>
      <EvaluatorDriftTable
        rows={rows}
        evaluatorId={evaluatorId}
        fixMut={fixMut}
      />
    </>
  );
}

function EvaluatorDriftTable({
  rows,
  evaluatorId,
  fixMut,
}: {
  rows: EvaluatorDriftRow[];
  evaluatorId: string;
  fixMut: ReturnType<typeof useFixDeploymentDrift>;
}) {
  return (
    <table className="mt-2 w-full text-[11px]">
      <thead>
        <tr className="border-b border-comb-light text-left text-muted-foreground">
          <th className="py-1 pr-2 font-medium">Deployment</th>
          <th className="py-1 pr-2 font-medium">Account</th>
          <th className="py-1 pr-2 font-medium">Status</th>
          <th className="py-1 pr-2 font-medium">Detail</th>
          <th className="py-1 pr-2 font-medium">Checked</th>
          <th className="py-1 pr-2 font-medium" />
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.deployment_id} className="border-b border-comb-light/50">
            <td className="py-1 pr-2 font-mono text-amber">
              {row.agent_name || row.deployment_id}
            </td>
            <td className="py-1 pr-2 text-muted-foreground">
              {row.account_name}
            </td>
            <td className="py-1 pr-2">
              <span
                className={cn(
                  "rounded-full px-1.5 py-0.5 text-[10px]",
                  row.status === "fix_failed"
                    ? "bg-red-100/60 text-red-700"
                    : "bg-amber-100/60 text-amber-800",
                )}
              >
                {row.status}
              </span>
            </td>
            <td
              className="max-w-[260px] truncate py-1 pr-2 text-muted-foreground"
              title={row.detail}
            >
              {row.detail}
            </td>
            <td className="py-1 pr-2 text-muted-foreground">
              {row.checked_at
                ? formatDistanceToNow(new Date(row.checked_at), {
                    addSuffix: true,
                  })
                : "-"}
            </td>
            <td className="py-1 pr-2">
              <Button
                size="xs"
                variant="outline"
                className="gap-1"
                disabled={fixMut.isPending}
                onClick={() =>
                  fixMut.mutate({ evaluatorId, deploymentId: row.deployment_id })
                }
              >
                <Wrench className="size-3" />
                Fix
              </Button>
            </td>
          </tr>
        ))}
      </tbody>
      {fixMut.isError && (
        <tfoot>
          <tr>
            <td colSpan={6} className="pt-1 text-[10px] text-destructive">
              {mutationErrorMessage(fixMut.error)}
            </td>
          </tr>
        </tfoot>
      )}
    </table>
  );
}
