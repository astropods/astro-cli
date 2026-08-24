import { useState } from "react";
import { Link } from "react-router";
import { useAcknowledgeAuditFinding, useAuditFindings } from "@/api/admin";
import type { AuditFinding } from "@/api/admin";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { cn, formatDateTime } from "@/lib/utils";

function severityClasses(sev: string): string {
  switch (sev) {
    case "error":
      return "bg-red-500/15 text-red-600 dark:text-red-400";
    case "warning":
      return "bg-amber-500/15 text-amber-600 dark:text-amber-400";
    default:
      return "bg-sky-500/15 text-sky-600 dark:text-sky-400";
  }
}

function Pill({ className, children }: { className: string; children: React.ReactNode }) {
  return (
    <span className={cn("rounded px-1.5 py-0.5 text-[10px] font-medium", className)}>
      {children}
    </span>
  );
}

function subjectLink(f: AuditFinding): string | null {
  if (f.check_name.startsWith("account.") || f.check_name.startsWith("billing.")) {
    return `/admin/accounts/${encodeURIComponent(f.subject_id)}`;
  }
  if (f.check_name.startsWith("deployment.")) {
    return `/admin/deployments/${encodeURIComponent(f.subject_id)}`;
  }
  return null;
}

function DetailChips({ detail }: { detail: string }) {
  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(detail || "{}") as Record<string, unknown>;
  } catch {
    return <span className="text-muted-foreground">{detail}</span>;
  }
  const entries = Object.entries(parsed);
  if (entries.length === 0) return <span className="text-muted-foreground">—</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {entries.map(([k, v]) => (
        <span key={k} className="rounded bg-muted px-1 py-0.5 text-[10px] text-muted-foreground">
          {k}={String(v)}
        </span>
      ))}
    </div>
  );
}

export function AuditPage() {
  const [includeResolved, setIncludeResolved] = useState(false);
  const [search, setSearch] = useState("");
  const { data, isLoading, error } = useAuditFindings(includeResolved);

  const findings = data?.findings?.filter((f) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      f.check_name.toLowerCase().includes(q) ||
      f.title.toLowerCase().includes(q) ||
      f.subject_label.toLowerCase().includes(q) ||
      f.subject_id.toLowerCase().includes(q) ||
      f.detail.toLowerCase().includes(q)
    );
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">System audit</h2>
          <p className="text-[10px] text-muted-foreground">
            Data integrity findings from the hourly sweep.{" "}
            {data && (
              <>
                {data.open_errors} errors · {data.open_warnings} warnings open.
              </>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant={includeResolved ? "default" : "outline"}
            size="sm"
            onClick={() => setIncludeResolved((v) => !v)}
          >
            {includeResolved ? "Hide resolved" : "Show resolved"}
          </Button>
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search findings..."
            className="w-56"
          />
        </div>
      </div>

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}

      <div className="overflow-x-auto rounded-lg glass">
        <table className="w-full text-[11px] whitespace-nowrap">
          <thead>
            <tr className="border-b border-glass-border-honey glass-subtle">
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Severity</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Check</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Subject</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Detail</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">First seen</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">Last seen</th>
              <th className="px-2 py-1.5 text-left font-medium text-muted-foreground">State</th>
              <th className="px-2 py-1.5 text-right font-medium text-muted-foreground">Actions</th>
            </tr>
          </thead>
          <tbody>
            {findings?.length === 0 && (
              <tr>
                <td colSpan={8} className="px-2 py-4 text-center text-muted-foreground">
                  {search ? "No matching findings." : "Nothing flagged. 🎉"}
                </td>
              </tr>
            )}
            {findings?.map((f) => (
              <FindingRow key={`${f.check_name}:${f.subject_id}`} finding={f} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function FindingRow({ finding: f }: { finding: AuditFinding }) {
  const acknowledge = useAcknowledgeAuditFinding();
  const link = subjectLink(f);

  return (
    <tr className="border-b border-glass-border-honey hover:bg-glass-light">
      <td className="px-2 py-1.5">
        <Pill className={severityClasses(f.severity)}>{f.severity}</Pill>
      </td>
      <td className="px-2 py-1.5" title={f.check_name}>
        {f.title || f.check_name}
      </td>
      <td className="px-2 py-1.5">
        {link ? (
          <Link to={link} className="hover:underline" title={f.subject_id}>
            {f.subject_label || f.subject_id}
          </Link>
        ) : (
          <span title={f.subject_id}>{f.subject_label || f.subject_id}</span>
        )}
      </td>
      <td className="px-2 py-1.5 whitespace-normal">
        <DetailChips detail={f.detail} />
      </td>
      <td className="px-2 py-1.5 text-muted-foreground">{formatDateTime(f.first_seen_at)}</td>
      <td className="px-2 py-1.5 text-muted-foreground">{formatDateTime(f.last_seen_at)}</td>
      <td className="px-2 py-1.5">
        {f.resolved_at ? (
          <Pill className="bg-muted text-muted-foreground">resolved</Pill>
        ) : f.acknowledged_at ? (
          <Pill className="bg-muted text-muted-foreground">acknowledged</Pill>
        ) : (
          <Pill className="bg-red-500/10 text-red-600 dark:text-red-400">open</Pill>
        )}
      </td>
      <td className="px-2 py-1.5 text-right">
        {!f.resolved_at && !f.acknowledged_at && (
          <Button
            variant="outline"
            size="sm"
            disabled={acknowledge.isPending}
            onClick={() =>
              acknowledge.mutate({ check_name: f.check_name, subject_id: f.subject_id })
            }
          >
            Acknowledge
          </Button>
        )}
      </td>
    </tr>
  );
}
